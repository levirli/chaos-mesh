# Chaos 实验生命周期

## Purpose

固化 Chaos Mesh 实验从「创建 → 注入 → 恢复」的完整生命周期行为契约，覆盖通用 pipeline 调度与 NetworkChaos 异步两阶段注入机制，作为现有实现的 reference 规范。

Chaos Mesh 把一次混沌实验的「创建 → 注入 → 恢复」拆成三层职责，每层只做一件事：

```
第 0 层  Schedule（可选）      到点把实验模板实例化成具体 Chaos CR
第 1 层  common pipeline       决定该不该注入/恢复 + 选目标 + 调度 Apply/Recover
第 2 层  chaosimpl            具体 chaos 实现：改 K8s 对象 / 下发子 CR / gRPC 调 daemon
```

三层状态分别给三类读者看，不要混淆：

| 状态 | 字段 | 取值 | 读者 |
| --- | --- | --- | --- |
| 控制器内部意图 | `status.experiment.desiredPhase` | `Run` / `Stop` | pipeline |
| 每个目标的实际进度 | `status.experiment.records[].phase` | `Not Injected` / `Injected` 及中间态 `*/*` | records 控制器 |
| 展示态 | `pkg/status/status.go` 计算 | `injecting` / `running` / `finished` / `paused` / `deleting` | dashboard/metrics |

恢复不是独立状态：它就是 `desiredPhase=Stop` 且 records 尚未全部回到 `Not Injected` 的中间过程。

## Requirements

### Requirement: Schedule 将实验模板实例化为具体 Chaos CR

Schedule 控制器 SHALL NOT 直接执行注入；它 SHALL 到点根据 `spec.scheduleItem` 调用 `SpawnNewObject` 生成具体 Chaos CR（PodChaos/NetworkChaos/…）或 Workflow，设置 OwnerReference 指向自身、打上 `managed-by=<schedule.Name>` 标签后 Create。真正的注入 MUST 由 common pipeline 接手。

#### Scenario: cron 到点未错过
- **WHEN** Schedule 的 `getRecentUnmetScheduleTime` 返回一个未满足的运行时刻，且未超过 `startingDeadlineSeconds`、`concurrencyPolicy` 允许
- **THEN** `SpawnNewObject` 生成具体 Chaos CR 并 Create，`schedule.status.lastScheduleTime` 更新为该时刻
- **AND** 控制器返回 `RequeueAfter` 等到下一次运行时刻

#### Scenario: 错过且超过 deadline
- **WHEN** 错过的运行时刻已超出 `startingDeadlineSeconds` 窗口
- **THEN** 不再补跑，记录 `MissedSchedule` 事件，Requeue 到下一次运行时刻

### Requirement: common pipeline 按固定顺序执行五步

每种 Chaos CR 的 Reconcile SHALL 经过同一条固定顺序流水线：`finalizers.InitStep` → `desiredphase.Step` → `condition.Step` → `records.Step` → `finalizers.CleanStep`。

#### Scenario: 某步要求 RequeueAfter
- **WHEN** 某个 step 返回 `RequeueAfter`（例如 desiredphase 设了 duration 倒计时）
- **THEN** pipeline 不立即返回，而是记录最早 deadline、继续执行后续 step，最后统一返回最短的 `RequeueAfter`

#### Scenario: 某步要求 Requeue
- **WHEN** 某个 step 返回 `Requeue: true`
- **THEN** pipeline 立即中断并返回，不再执行后续 step

### Requirement: DesiredPhase 决定期望状态

`desiredphase.Reconciler` SHALL 按固定优先级计算 `status.experiment.desiredPhase` 以决定本次 reconcile 注入还是恢复。优先级从高到低：删除 > one-shot > duration 到期 > pause > 默认运行。

#### Scenario: duration 到期触发自动恢复
- **WHEN** `obj.DurationExceeded(now)` 为真（`creationTimestamp + spec.duration < now`）
- **THEN** `desiredPhase` 设为 `Stop`，记录 `TimeUp` 事件
- **AND** 当 duration 未到期时返回 `RequeueAfter = 剩余时间`，使 controller-runtime 在到期时自动唤醒

#### Scenario: 删除触发恢复
- **WHEN** 对象带有 deletionTimestamp（`IsDeleted()` 为真）
- **THEN** `desiredPhase` 设为 `Stop`，驱动后续 records 走 Recover

#### Scenario: pause 暂停
- **WHEN** annotation `experiment.chaos-mesh.org/pause=true`（`IsPaused()` 为真）
- **THEN** `desiredPhase` 设为 `Stop`，记录 `Paused` 事件；取消 pause 后回到 `Run`

#### Scenario: one-shot 永远运行
- **WHEN** `IsOneShot()` 为真（如 pod-kill / container-kill）
- **THEN** `desiredPhase` 始终为 `Run`，不受 duration 影响

### Requirement: records 循环按 phase 单调推进且不可跳步

`records.Reconciler` SHALL 对每条 record 依据 `(desiredPhase, 当前 phase)` 决定调 `Impl.Apply` 还是 `Impl.Recover`，且 phase MUST 沿固定环推进：`Not Injected → Not Injected/* → Injected → Injected/* → Not Injected`，不允许跨步。

```
desiredPhase=Run  且 phase≠Injected  →  以"Not Injected"开头? Apply : Recover
desiredPhase=Stop 且 phase≠NotInjected →  以"Not Injected"开头? Apply : Recover
```

#### Scenario: 期望运行但停在恢复中间态
- **WHEN** `desiredPhase=Run` 且 record 当前 phase 为 `Injected/Wait`（上次 Stop 留下的半截 recover）
- **THEN** 不直接 Apply，而是先调 `Impl.Recover` 把在途恢复走完到 `Not Injected`，下一轮再 Apply

#### Scenario: Apply 成功
- **WHEN** `Impl.Apply` 返回 `Injected`
- **THEN** `record.Phase=Injected`、`InjectedCount++`，记录 apply 成功事件

#### Scenario: Recover 成功
- **WHEN** `Impl.Recover` 返回 `NotInjected`
- **THEN** `record.Phase=NotInjected`、`RecoveredCount++`，记录 recover 成功事件

### Requirement: 注入意图与执行分离（chaosimpl 三种形态）

`records.Reconciler` 调到的 `Impl.Apply/Recover` SHALL 按是否需要 daemon 分为三类，但接口 MUST 统一：
- 形态 A：直接操作 K8s 对象（如 pod-failure 改 image），不走 daemon。
- 形态 B：异步子 CR（如 NetworkChaos 写 PodNetworkChaos），返回中间态 phase。
- 形态 C：直接 gRPC 调 chaos-daemon（如 stress/io/http/dns/block/container-kill）。

#### Scenario: 直接操作 K8s 对象
- **WHEN** chaosimpl 为 pod-failure
- **THEN** Apply 把容器 image 存进 annotation 并替换成 PauseImage 后 Patch Pod；Recover 从 annotation 还原原 image 并删除 annotation

#### Scenario: 直接 gRPC 调 daemon
- **WHEN** chaosimpl 为 stress/io/http 等
- **THEN** Apply 经 `ChaosDaemonClientBuilder.Build` 拿到目标 Pod 同节点的 daemon client，发 gRPC；daemon 进入目标 Pod namespace 执行实际注入

### Requirement: NetworkChaos 采用异步两阶段与 generation 握手收敛

NetworkChaos 的 `Apply`/`Recover` SHALL NOT 直接注入；它 SHALL 只把意图（tc/iptables/ipset 规则）写进子 CR `PodNetworkChaos`，记下写入时的 generation，返回中间态 phase `Not Injected/Wait` 或 `Injected/Wait` 等待。真正注入 MUST 发生在 `podnetworkchaos` 控制器里，完成后回写 `ObservedGeneration`。父 CR SHALL 被子 CR 的 watch 唤醒再次 reconcile，看到 generation 追上才把 phase 翻到稳态。

phase 环（`Wait` 是 `*` 的实现）：

```
Not Injected --Apply写意图--> Not Injected/Wait --daemon追上gen--> Injected
     ^                                                            |
     |                                              Recover写意图  |
     +---------------- daemon追上gen <--- Injected/Wait <--------+
```

generation 握手：父 `networkchaos.status.instances[record.Id] = G`（我最后写入意图时的 generation），子 `podnetworkchaos.status.observedGeneration = H`（daemon 实际应用到的 generation）。完成 ⟺ `H >= G`。

#### Scenario: Apply 第一阶段写意图
- **WHEN** record phase 为 `Not Injected` 且 desiredPhase 为 `Run`
- **THEN** `WithInit` → `Clear(source)` → `Append` tc/iptables/ipset 规则 → `Commit` 写入/创建子 CR
- **AND** 记录 `instances[record.Id] = Commit 返回的 generation`，phase 置为 `Not Injected/Wait`，本轮结束
- **AND** 因未返回 error，records 控制器不 requeue，等待子 CR watch 唤醒

#### Scenario: Apply 第二阶段确认完成
- **WHEN** record phase 为 `Not Injected/Wait`，读到子 CR
- **IF** `observedGeneration >= instances[record.Id]`
- **THEN** 返回 `Injected`
- **IF** 子 CR `FailedMessage` 非空
- **THEN** 返回错误，phase 保持 `Not Injected/Wait`
- **ELSE** 继续返回 `Not Injected/Wait`

#### Scenario: Recover 第一阶段写意图
- **WHEN** record phase 为 `Injected` 且 desiredPhase 为 `Stop`
- **THEN** `WithInit` → `Clear(source)` 摘掉自己写的规则 → `Commit`
- **AND** 记录 `instances[record.Id] = 新 generation`，phase 置为 `Injected/Wait`

#### Scenario: Recover 第二阶段确认完成
- **WHEN** record phase 为 `Injected/Wait`，读到子 CR
- **IF** `observedGeneration >= instances[record.Id]`
- **THEN** 返回 `NotInjected`

#### Scenario: 子 CR 不存在时安全跳过
- **WHEN** 等待阶段读到子 CR NotFound 或 `is being terminated`
- **THEN** 返回 `NotInjected`，避免 pod 已删除时死锁

### Requirement: PodNetworkChaos 按 Pod 聚合，多实验以 Source 区分

一个 Pod SHALL 只有一个 `PodNetworkChaos`（名字 = Pod 名，OwnerReference 指向 Pod），但可被多个 NetworkChaos 同时盯上。所有写入的规则（`RawIPSet`/`RawIptables`/`RawTrafficControl`）MUST 带 `Source` 字段（`networkchaos.Namespace + "/" + networkchaos.Name`）。`Clear(source)` SHALL 只删 `Source` 匹配自己的规则，使多个实验的 recover 互不干扰、各自幂等。

#### Scenario: 多实验共存时单独 recover
- **WHEN** NetworkChaos A 与 B 同时注入同一 Pod 的 PodNetworkChaos，A 进入 recover
- **THEN** A 的 `WithInit` → `Clear("nsA/chaos-a")` 只摘除 A 的规则，B 的规则保持不变

#### Scenario: Pod 不存在时 commit
- **WHEN** `CreateNewPodNetworkChaos` 时 Pod NotFound 或非 Running
- **THEN** 返回 `ErrPodNotFound` / `ErrPodNotRunning`，调用方视作已恢复返回 `NotInjected`

### Requirement: direction × selectorKey 决定规则写在哪一端 Pod

NetworkChaos 的 `GetSelectorSpecs()` SHALL 产出两组 record：`.`（source pods）与 `.Target`（target pods）。`Direction`（To/From/Both）MUST 决定规则写到哪一端、用哪个 device 与 ipset 后缀。

| | direction: To | direction: From | direction: Both |
| --- | --- | --- | --- |
| record `.`（source） | 在 source pod 写，device=`Spec.Device`，ipset 后缀 `tgt` | 不处理（返回 Injected） | 在 source pod 写（To 部分） |
| record `.Target`（target） | 不处理（返回 Injected） | 在 target pod 写，device=`Spec.TargetDevice`，ipset 后缀 `src` | 在 target pod 写（From 部分） |

#### Scenario: direction=To 的 partition
- **WHEN** `Action=partition`、`Direction=To`
- **THEN** `.` records 在 source pod 上写 OUTPUT 链 DROP（匹配 target IPSet）；`.Target` records 直接返回 `Injected`（无事可做）

#### Scenario: partition 处理选择器重叠
- **WHEN** 某 `.Target` pod 同时也出现在 `.` 选择器里
- **THEN** partition 对该 `.Target` 改用 `Build`（不 Clear），避免冲掉 `.` 刚写的规则

### Requirement: 恢复由 duration 到期 / pause / 删除三源触发

三个触发源 SHALL 最终汇到同一条路径：让 `desiredPhase` 变成 `Stop`，records 控制器据此对每条非 `NotInjected` 的 record 调 `Impl.Recover`，全部回到 `NotInjected` 后 `AllRecovered=True`。

#### Scenario: duration 到期唤醒
- **WHEN** desiredphase 在 duration 未到期时返回 `RequeueAfter = 剩余时间`
- **THEN** controller-runtime 在到期时自动 reconcile，算出 `Stop`，触发 Recover

#### Scenario: 删除时先恢复再删
- **WHEN** 用户删除 Chaos CR
- **THEN** 因 `chaos-mesh/records` finalizer 存在，对象以 deleting 状态保留并被继续 reconcile
- **AND** desiredphase 算出 `Stop` → records 调 Recover → 全部 `NotInjected` 后移除 finalizer → 对象真正删除

### Requirement: Finalizer 保证恢复完成

`chaos-mesh/records` finalizer SHALL 由 `finalizers.InitStep` 在非删除态添加，由 `finalizers.CleanStep` 在删除态且所有 records 都 `NotInjected` 时移除。删除操作 MUST NOT 在恢复完成前生效。

#### Scenario: 强制清理
- **WHEN** annotation `chaos-mesh.chaos-mesh.org/cleanFinalizer=forced`
- **THEN** 直接清空 finalizers，绕过恢复等待（用于卡死场景）

### Requirement: chaos-daemon 通过目标 Pod namespace 执行注入

controller SHALL NOT 进入 Pod，而是 SHALL 找目标 Pod 同节点的 chaos-daemon（按 `targetPod.Spec.NodeName` 匹配 chaos-daemon EndpointSlice）发 gRPC（默认端口 31767）。请求 MUST 带 `EnterNS:true`、`ContainerId`、`PodUid`。daemon SHALL 经容器运行时拿到容器 PID（失败则 fallback 到 sandbox PID，因为网络 namespace 由 pause 容器持有），用 `bpm.SetNS(pid, NetNS|MountNS|PidNS)` 把命令包装成 `nsexec -n /proc/<pid>/ns/net -- <cmd>` 执行实际注入。

不同 chaos 进入不同 namespace：

| chaos | namespace | 实际命令 |
| --- | --- | --- |
| network（tc/iptables/ipset） | net | `tc` / `iptables` / `ipset` |
| dns | mnt | 改 `/etc/resolv.conf` |
| io | mnt + pid | `toda` |
| http | pid + net | `tproxy` |
| stress | pid | `stress-ng` + cgroup attach |

#### Scenario: 进入目标 Pod 网络命名空间
- **WHEN** controller 发来 `EnterNS:true` + `ContainerId`
- **THEN** daemon `GetPidFromContainerID` 拿 PID（失败 fallback `GetSandboxPidFromPodUID`），构造 `/usr/local/bin/nsexec -n /proc/<pid>/ns/net -- tc qdisc ...` 在目标 netns 内执行

### Requirement: condition 据 records 刷新展示态条件

`condition.Reconciler` SHALL 据 `status.experiment.records` 与 pause 状态刷新 `status.conditions`：`Selected`（records 非 nil）、`AllInjected`（全部 Injected）、`AllRecovered`（全部 NotInjected）、`Paused`（`IsPaused()`）。`pkg/status/status.go` SHALL 据此计算展示态。

#### Scenario: 计算展示态
- **WHEN** `IsDeleted()` → `deleting`；`Paused=True` → `paused`；`IsChaosFinished()` → `finished`；`Selected=True` 且 `AllInjected=True` → `running`；否则 `injecting`
- **THEN** 该展示态反映给 dashboard/metrics
