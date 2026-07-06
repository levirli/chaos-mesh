# Chaos 实验生命周期 — 设计参考

本文件是 `openspec/specs/chaos-experiment-lifecycle/spec.md` 的配套设计参考。spec 用 Requirement/Scenario 固化行为契约；本文承载契约无法表达的非契约性内容：架构图、状态机、端到端时序、代码 `file:line` 锚点。**本文描述的是现有实现的"现状"，不是待实现的设计。**

## Context

Chaos Mesh 是云原生混沌工程平台，核心由两个组件协作完成注入：

- **chaos-controller-manager**：调度与编排，决定"该不该注入/恢复"，但不直接进 Pod。
- **chaos-daemon**：每个节点一个 DaemonSet，特权运行，真正进入目标 Pod 的 namespace 执行 `tc`/`iptables`/`stress-ng` 等命令。

两者通过 gRPC（默认端口 31767）通信。controller 找目标 Pod **同节点**的 daemon 发请求，请求带 `EnterNS:true` + `ContainerId` + `PodUid`，daemon 据此进入目标 namespace 执行。

实验生命周期被刻意拆成三层职责，每层只做一件事。这套分层是理解全部机制的总纲。

## Goals / Non-Goals

**Goals:**
- 用图把三层抽象、phase 状态机、generation 握手、direction 矩阵、恢复汇流等机制可视化。
- 给出一次实验从生到灭的完整端到端时序走查。
- 提供关键代码 `file:line` 锚点索引，让读者能从文档跳到实现。

**Non-Goals:**
- 不重复 spec 已固化的行为契约（WHEN/THEN 在 spec 里）。
- 不覆盖 selector 细节、Workflow 编排、webhook 校验——这些是独立话题。
- 不指导如何修改代码；本文是 reference，不是实现计划。

## Decisions

### D1. 三层抽象职责划分

```
┌─────────────────────────────────────────────────────────────────┐
│  第 0 层  Schedule（可选）                                       │
│  到点把"实验模板"实例化成具体 Chaos CR                            │
│  controllers/schedule/cron                                       │
└─────────────────────────────────────────────────────────────────┘
                              │ SpawnNewObject + Create
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  第 1 层  common pipeline（通用 reconciler，所有 chaos 共用）    │
│  决定"该不该注入/恢复" + 选目标 + 调度 Apply/Recover              │
│  controllers/common/{finalizers,desiredphase,condition,records}  │
└─────────────────────────────────────────────────────────────────┘
                              │ Impl.Apply / Impl.Recover
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  第 2 层  chaosimpl（每种 chaos 的具体实现）                     │
│  ① 简单类型：直接改 K8s 对象（pod-kill / pod-failure）          │
│  ② 复杂类型：下发子 CR（PodNetworkChaos / PodIOChaos …）        │
│  ③ daemon 类型：gRPC 调 chaos-daemon 进 netns 执行              │
└─────────────────────────────────────────────────────────────────┘
```

**为什么分三层**：第 1 层把"该不该注入、选哪些目标、phase 怎么推进"这种所有 chaos 共用的逻辑抽出来，避免每种 chaos 重写一遍；第 2 层只关心"对单个目标具体怎么动手"。第 0 层可选，没有它用户直接建 Chaos CR 也能跑。

### D2. 三层状态分别给三类读者看

这是最容易混淆的点——"状态"有三套：

```
 控制器内部意图          每个目标的实际进度         Dashboard/Metrics 展示
 status.desiredPhase     records[].phase           pkg/status/status.go
 ─────────────────       ──────────────────        ─────────────────────
 Run / Stop              Not Injected              injecting
                         Injected                  running
                         (中间态: Not Injected/*   finished
                          / Injected/* )           paused
                                                  deleting
```

**关键认知**：恢复不是独立状态。它就是 `desiredPhase=Stop` 且 records 尚未全部回到 `Not Injected` 的中间过程。dashboard 没有单独的 "recovering"——那个过程显示为 `injecting`（若 desiredPhase 还是 Run 但有未注入目标）或直接走向 `finished`。

### D3. common pipeline 固定五步与 Requeue 语义

`controllers/common/step.go` 定义每种 Chaos CR 的 Reconcile 顺序：

```
 Pipeline.Reconcile(ctx, req)
   ├─① finalizers.InitStep      非删除态 → 确保带 chaos-mesh/records finalizer
   ├─② desiredphase.Step        计算 desiredPhase（Run/Stop），返回 RequeueAfter=剩余duration
   ├─③ condition.Step           据 records 刷 Selected/AllInjected/AllRecovered/Paused
   ├─④ records.Step   ★核心     selector 选目标 → 建空 record → 调 Apply/Recover
   └─⑤ finalizers.CleanStep     删除态 & 全部NotInjected → 移除 finalizer
```

**Requeue 语义**（`controllers/common/pipeline/pipeline.go`）是理解自动恢复的关键：
- step 返回 `Requeue: true` → pipeline **立即中断**返回，不跑后续。
- step 返回 `RequeueAfter` → pipeline 记录最早 deadline、**继续往下跑**，最后统一返回最短 `RequeueAfter`。

后者解释了为什么 desiredphase 设了 duration 倒计时后，pipeline 仍会继续执行 records 那一步——倒计时只决定"何时再来一次"，不阻塞本轮。

### D4. desiredPhase 优先级

`controllers/common/desiredphase/controller.go` 的 `CalcDesiredPhase` 按固定优先级：

```
IsDeleted()           → Stop   （正在被删除）
IsOneShot()           → Run    （pod-kill/container-kill 永远 Run）
DurationExceeded(now) → Stop   （duration 到期，发 TimeUp 事件）
                       → 否则记 requeueAfter = 剩余时间
IsPaused()            → Stop   （annotation pause=true）
default               → Run    （发 Started 事件）
```

**自动恢复的触发源**就在这里：duration 到期 → 下次 reconcile 算出 `Stop` → records 看到 `Stop` 走 Recover 分支。desiredphase 通过返回 `RequeueAfter=剩余时间`，让 controller-runtime 在到期时自动唤醒，无需轮询。

### D5. records 循环的 phase 单调环与"不可跳步"

`controllers/common/records/controller.go:123-149` 的决策表是通用 pipeline 与具体 chaosimpl 的对接核心：

```
desiredPhase=Run  且 phase≠Injected  →  以"Not Injected"开头? Apply : Recover
desiredPhase=Stop 且 phase≠NotInjected →  以"Not Injected"开头? Apply : Recover
```

phase 必须沿固定环推进，不可跨步：

```
          ┌─────────────────────────────────────────────────────┐
          │                                                     │
          ▼                                                     │
   ┌──────────────┐                ┌─────────────────┐           │
   │ Not Injected │  ──Apply────►  │ Not Injected/*  │           │
   │  (初始)      │                │   (中间态)       │           │
   └──────────────┘                └────────┬────────┘           │
          ▲                                 │                    │
          │                          (具体实现推进)              │
          │                                 ▼                    │
          │                        ┌──────────────┐             │
          │   (具体实现推进)         │   Injected   │             │
          │ ┌──────────────────────│  (稳态)       │             │
          │ │                      └──────┬───────┘             │
          │ ▼                             │ Recover              │
   ┌──────────────┐                ┌──────────────┐             │
   │ Not Injected │  ◄──Recover──  │  Injected/*   │ ───────────┘
   │ (恢复完成)   │                │   (中间态)     │
   └──────────────┘                └──────────────┘
```

**为什么不能跳步**：注释（`records/controller.go:123-126`）举例——若 phase 停在 `Not Injected/*`（Apply 的中间态）时收到 `Recover`，必须先调 Apply 走完到 `Injected`，下一轮才能 Recover。直接 Recover 会跳过未完成的 Apply，留下不一致状态。这强制所有实现沿环单调推进。

### D6. chaosimpl 三种形态

`records.Reconciler` 调到的 `Impl.Apply/Recover` 按是否需要 daemon 分三类，接口统一：

| 形态 | 代表 | Apply 做什么 | 是否走 daemon |
| --- | --- | --- | --- |
| A 直接改 K8s 对象 | pod-failure | image 存 annotation → 替换 PauseImage → Patch Pod | 否 |
| A' one-shot | pod-kill / container-kill | Delete Pod / gRPC kill 容器；Recover 空操作 | container-kill 走 daemon |
| B 异步子 CR | NetworkChaos | 写 PodNetworkChaos 意图，返回 Wait 中间态 | 间接（子CR控制器走） |
| C 直接 gRPC | stress/io/http/dns/block | 直接拿 daemon client 发 RPC | 是 |

**形态 B 是最复杂的**，下面单独展开（D7-D11）。

### D7. NetworkChaos 异步两阶段 + generation 握手

NetworkChaos 的 `Apply`/`Recover` **本身不注入**。它只把意图（tc/iptables/ipset 规则）写进子 CR `PodNetworkChaos`，记下写入时的 generation，返回中间态 phase 等待。真正注入发生在 `podnetworkchaos` 控制器里，完成后回写 `ObservedGeneration`。父 CR 被子 CR 的 watch 唤醒再次 reconcile，看到 generation 追上才翻 phase 到稳态。

NetworkChaos 把通用环的 `*` 实现为 `Wait`（`trafficcontrol/impl.go:46-48` / `partition/impl.go:54-57`）：

```
Not Injected --Apply写意图--> Not Injected/Wait --daemon追上gen--> Injected
     ^                                                            |
     |                                              Recover写意图  |
     +---------------- daemon追上gen <--- Injected/Wait <--------+
```

**generation 握手**——两个水位线分属两个 CR：

```
   父 NetworkChaos                     子 PodNetworkChaos
   ────────────────                    ────────────────────
   status.instances[record.Id] = G    status.observedGeneration = H
        │                                      │
        │  "我最后写入意图时的 generation"        │  "daemon 实际应用到的 generation"
        │                                      │
        └──────── 完成 ⟺ H >= G ────────────────┘
```

Apply 两阶段（`trafficcontrol/impl.go`）：
- **写意图**（phase=`Not Injected`）：`WithInit` → `Clear(source)` → `Append` 规则 → `Commit` 写/建子CR → 记 `instances[record.Id]=generation` → 返回 `Not Injected/Wait`（`impl.go:133-140`）。
- **确认完成**（phase=`Not Injected/Wait`）：读子CR，`observedGeneration >= instances[record.Id]` → 返回 `Injected`（`impl.go:95-99`）；`FailedMessage` 非空 → 返回错误；否则继续 `Wait`。

Recover 两阶段对称（`trafficcontrol/impl.go`）：写意图返回 `Injected/Wait`（`impl.go:236-250`），确认完成返回 `NotInjected`（`impl.go:186-211`）。

**谁来唤醒等待**：写意图那步不返回 error，records 控制器 `needRetry=false`，`Requeue:false`。唤醒靠 `common/fx.go` 里 watch 子 CR（`Controlls: []client.Object{&v1alpha1.PodNetworkChaos{}}`，见 `networkchaos/impl.go:44`）。daemon 更新子CR `ObservedGeneration` → status 变化触发 watch → 反查并重新 reconcile 父 CR。**异步收敛是事件驱动，不是轮询。**

### D8. PodNetworkChaos 按 Pod 聚合，多实验以 Source 区分

一个 Pod 只有一个 `PodNetworkChaos`（`podnetworkchaosmanager.go:128`：`chaos.Name = m.Key.Name`，OwnerReference 指向 Pod），但可被多个 NetworkChaos 同时盯上。区分谁写的规则靠 `Source` 字段：

```go
source := networkchaos.Namespace + "/" + networkchaos.Name   // 每个 NetworkChaos 一个唯一 source
```

所有写入子 CR 的规则（`RawIPSet`/`RawIptables`/`RawTrafficControl`）都带 `Source` 标记。`Clear(source)`（`transaction.go:42-68`）只删 `Source == 自己` 的规则：

```
   NetworkChaos A (nsA/chaos-a) ──┐
   NetworkChaos B (nsB/chaos-b) ──┼──► 同一个 PodNetworkChaos (per pod)
                                   │     spec.ipsets/iptables/tcs[]
                                   │     每条带 Source 区分归属
   A recover → Clear("nsA/chaos-a") 只摘 A 的规则，B 的不动
```

**为什么 recover 用 `WithInit`（先 Clear 再 Append）幂等且互不干扰**：多个实验共存时，各自 recover 只清理自己的 source，不会误删别人写的规则。`WithInit` 见 `builder.go:66-80`。

### D9. direction × selectorKey 规则归属矩阵

NetworkChaos 的 `GetSelectorSpecs()` 产出两组 record：`.`（source pods）与 `.Target`（target pods）。`Direction`（To/From/Both）决定规则写到哪一端、用哪个 device 与 ipset 后缀：

| | direction: To | direction: From | direction: Both |
| --- | --- | --- | --- |
| record `.`（source） | 在 source pod 写，device=`Spec.Device`，ipset 后缀 `tgt` | 不处理（返回 Injected） | 在 source pod 写（To 部分） |
| record `.Target`（target） | 不处理（返回 Injected） | 在 target pod 写，device=`Spec.TargetDevice`，ipset 后缀 `src` | 在 target pod 写（From 部分） |

**含义**：一个 NetworkChaos 实验实际产生 N+M 个独立 record（N 个 source + M 个 target），各自走自己的两阶段握手。source 端和 target 端的注入是并行收敛的，互不阻塞。矩阵代码见 `trafficcontrol/impl.go:119-172` 与 `partition/impl.go:144-242`。

### D10. partition 处理选择器重叠

trafficcontrol 在 `.` 和 `.Target` 两个分支都用 `WithInit`（先 Clear）。partition 对 `.Target` 多了一层判断（`partition/impl.go:118-142`）：若某 `.Target` pod 同时也出现在 `.` 选择器里（即同一 pod 既是源又是目标），则改用 `Build`（不 Clear），避免 `.Target` 的 Clear 把 `.` 刚写的规则冲掉。

**这是处理选择器重叠的防御逻辑**。trafficcontrol 没做这层处理，说明两者对重叠场景的鲁棒性不同——一个值得注意的差异点。

### D11. 子 CR 控制器的 observedGeneration 同步

`controllers/podnetworkchaos/controller.go` 是真正调 daemon 的执行器。关键逻辑：
- 幂等跳过：`Generation <= ObservedGeneration && FailedMessage==""` 直接返回（`controller.go:69`）。
- 顺序执行：`SetIPSets` → `SetIptables` → `SetTcs`，各自 gRPC 调 daemon（`controller.go:146-176`）。
- 回写 status：defer 里用 `Status().Update()` 写 `ObservedGeneration` 与 `FailedMessage`（`controller.go:88-119`）。

**为什么用 `Status().Update()` 而非 `Update()`**：走 status subresource，保证子控制器的 status 更新不会误改父 CR 的 generation，握手语义干净。

### D12. 恢复三触发源汇流与 finalizer 兜底

三个触发源最终都汇到同一条路径——让 `desiredPhase=Stop`，records 据此对每条非 `NotInjected` 的 record 调 `Impl.Recover`：

```
触发源                          机制
─────────────────────────────  ──────────────────────────────
① duration 到期                desiredphase 算出 Stop（发 TimeUp）
                               + requeueAfter 倒计时唤醒
② pause annotation=true        desiredphase 算出 Stop（发 Paused）
③ 删除 Chaos CR                IsDeleted() → Stop
                               + finalizer 阻止对象被真正删除
                                  │
                                  ▼
            records.Reconciler 看到 desiredPhase=Stop
            → 对每条非 NotInjected 的 record 调 Impl.Recover
                                  │
                                  ▼
            全部 record 回到 NotInjected
            → condition: AllRecovered=True
            → (场景③) finalizers.CleanStep 移除 chaos-mesh/records
              → Kubernetes 真正删除对象
```

**Finalizer 是恢复的最终保证**（`controllers/common/finalizers/controller.go`）：`chaos-mesh/records` 让"删除 CR"不立即生效，对象以 deleting 状态继续被 reconcile，直到所有目标 Recover 完才放行删除。卡死时可用 annotation `chaos-mesh.chaos-mesh.org/cleanFinalizer=forced` 强制清理。

### D13. chaos-daemon 经 nsexec 进入目标 namespace

controller 不进 Pod，找目标 Pod 同节点的 daemon 发 gRPC。daemon 端调用链：

```
chaosimpl
  └─ ChaosDaemonClientBuilder.Build(ctx, targetPod, namespacedName)
       └─ FindDaemonIP: 按 targetPod.Spec.NodeName 找同 node 的
                       chaos-daemon EndpointSlice 地址
       └─ grpc.Dial(daemonIP:31767) → pb.NewChaosDaemonClient
            │  请求带 EnterNS:true, ContainerId, PodUid
            ▼
chaos-daemon (pkg/chaosdaemon/*_server.go)
  ├─ crClient.GetPidFromContainerID(containerId)   拿容器主进程 PID
  │     └─ 失败则 fallback: GetSandboxPidFromPodUID(podUid)
  │        （网络 namespace 由 pause 容器持有，业务容器共享）
  ├─ bpm.DefaultProcessBuilder(cmd, args...).SetNS(pid, NetNS|MountNS|PidNS)
  └─ 实际执行：
       /usr/local/bin/nsexec -n /proc/<pid>/ns/net -- tc qdisc add ...
```

**`nsexec` 是 Chaos Mesh 自己的辅助二进制**，用 `setns(2)` 进入目标 namespace 后 exec 真正的注入命令。不同 chaos 进不同 namespace：

| chaos | namespace | 实际命令 |
| --- | --- | --- |
| network（tc/iptables/ipset） | net | `tc` / `iptables` / `ipset` |
| dns | mnt | 改 `/etc/resolv.conf` |
| io | mnt + pid | `toda` |
| http | pid + net | `tproxy` |
| stress | pid | `stress-ng` + cgroup attach |
| block | （经 chaos-driver） | ioem scheduler 注入延迟 |

namespace 路径常量见 `pkg/bpm/bpm.go`（`GetNsPath` → `/proc/<pid>/ns/<type>`），命令包装见 `pkg/bpm/build_linux.go`。

### D14. 端到端时序走查

以 `partition, direction: To`、2 个 source pod、1 个 target pod、duration=30s 为例：

```
t0  创建 NetworkChaos, duration=30s
    records pipeline:
      desiredphase → Run
      selector → 生成 3 条 record:2 个 SelectorKey="."(src1,src2),1 个 ".Target"(tgt1)
                 全部 Phase=NotInjected

t1  records.Reconcile 遍历每条 record:
    ┌─ src1 (".")  desiredPhase=Run, phase=NotInjected → Apply
    │    WithInit("ns/nc", src1) → Clear + Append DROP 规则 → Commit
    │    PodNetworkChaos(src1) 创建, generation=1
    │    Instances["src1"]=1, phase → "Not Injected/Wait"
    ├─ src2 (".")  同上 → PodNetworkChaos(src2) gen=1, phase → Wait
    └─ tgt1 (".Target") direction=To → 不处理, phase → Injected   ★ 直接稳态

t2  podnetworkchaos 控制器被 watch 触发(src1/src2 的子 CR 变化):
    对每个子 CR: Generation(1) > ObservedGeneration(0) → 执行
      ChaosDaemonClientBuilder.Build(pod) → gRPC → daemon
      daemon: GetPidFromContainerID → nsexec -n /proc/<pid>/ns/net -- iptables ...
      成功后回写 ObservedGeneration=1

t3  子 CR status 变化 → watch 反查父 NetworkChaos → 再次 records.Reconcile:
    src1: phase="Not Injected/Wait" → Apply → 读子CR → ObservedGen(1)>=Instances(1) ✓
           → phase = Injected   (InjectedCount++)
    src2: 同上 → Injected
    tgt1: 已是 Injected
    → condition: AllInjected=True → dashboard 状态 running

    ⏰ 同时 desiredphase 设了 requeueAfter=30s

t4  30s 到期 → desiredphase → Stop (TimeUp 事件)

t5  records.Reconcile: desiredPhase=Stop, phase=Injected → 以"Not Injected"开头?否 → Recover
    src1: Recover, phase=Injected(非Wait) → WithInit → Clear("ns/nc") 摘掉自己的 DROP → Commit
          子 CR generation=2, Instances["src1"]=2, phase → "Injected/Wait"
    src2: 同上 → Injected/Wait
    tgt1: phase=Injected, direction=To 下 tgt1 其实没写过规则 →
          WithInit → Clear(空) → Commit → gen 升 → Injected/Wait
          (幂等:即使没规则,Clear+Commit 也照走,daemon 重新应用空规则集 = 清理)

t6  podnetworkchaos 控制器:Generation(2)>Observed(1) → daemon 执行 iptables 删链
    → 回写 ObservedGeneration=2

t7  watch 反查父 → records.Reconcile:
    各 record: phase="Injected/Wait" → Recover → 读子CR → Observed(2)>=Instances(2) ✓
              → phase = NotInjected  (RecoveredCount++)
    → condition: AllRecovered=True → finished
    (若此时 CR 被删 → finalizers.CleanStep 移除 records finalizer → 真正删除)
```

## Risks / Trade-offs

- **[异步收敛依赖 watch 可靠性]** NetworkChaos 的两阶段收敛靠子 CR 的 status watch 唤醒父 CR。若 watch 事件丢失，父 CR 可能卡在 `Wait` 中间态。→ 缓解：records 控制器对 Apply/Recover 不返回 error 时 `Requeue:false`，但 controller-runtime 的 informer 有 resync 机制兜底；且 desiredphase 的 `RequeueAfter`（duration 倒计时）会在到期时强制重新 reconcile。
- **[选择器重叠的鲁棒性差异]** partition 处理了 `.Target` 与 `.` 重叠（D10），trafficcontrol 没有。重叠场景下 trafficcontrol 可能出现规则被误 Clear。→ 已在 spec D10 标注为已知差异点，未做修复（reference 性质，不改代码）。
- **[generation 握手是 per-record 的]** `instances[record.Id]` 每个 record 一个水位线，N 个目标 N 条独立握手。慢目标不阻塞快目标，但全部完成需等最慢的。→ 设计上的正确取舍，非风险。
- **[本文是现状描述，可能随代码演进过时]** 文中 `file:line` 锚点基于本仓库当前 develop 分支。代码重构后锚点会漂移。→ 缓解：锚点同时标注文件路径与函数名，即使行号漂移也能定位。

## 关键代码锚点索引

| 机制 | 文件 | 关键位置 |
| --- | --- | --- |
| controller 模块入口 | `controllers/fx.go` | `Module` |
| 为每类 ChaosImplPair 建 pipeline | `controllers/common/fx.go` | `Bootstrap` |
| pipeline 五步顺序 | `controllers/common/step.go` | `AllSteps` |
| Requeue/RequeueAfter 语义 | `controllers/common/pipeline/pipeline.go` | `Reconcile` |
| desiredPhase 优先级计算 | `controllers/common/desiredphase/controller.go` | `CalcDesiredPhase` |
| records phase 决策表 | `controllers/common/records/controller.go` | `:123-149` |
| finalizer init/clean | `controllers/common/finalizers/controller.go` | `InitReconciler`/`CleanReconciler` |
| Schedule cron 调度 | `controllers/schedule/cron/controller.go` | `Reconcile` |
| NetworkChaos action 分发 | `controllers/chaosimpl/networkchaos/impl.go` | `NewImpl` |
| trafficcontrol Wait 常量 | `controllers/chaosimpl/networkchaos/trafficcontrol/impl.go` | `:46-48` |
| trafficcontrol Apply 写意图 | 同上 | `:133-140` |
| trafficcontrol Apply 确认 | 同上 | `:95-99` |
| trafficcontrol Recover | 同上 | `:186-211, :236-250` |
| partition 选择器重叠处理 | `controllers/chaosimpl/networkchaos/partition/impl.go` | `:118-142` |
| PodNetworkManager.Commit | `controllers/chaosimpl/networkchaos/podnetworkchaosmanager/podnetworkchaosmanager.go` | `Commit`/`CreateNewPodNetworkChaos:128` |
| WithInit / Build | 同目录 `builder.go` | `:51, :66` |
| Clear(source) 逻辑 | 同目录 `transaction.go` | `:42-68` |
| 子 CR 执行器（调 daemon） | `controllers/podnetworkchaos/controller.go` | `Reconcile`/`:69,88-119,146-176` |
| daemon client builder | `controllers/utils/chaosdaemon/chaosdaemon.go` | `FindDaemonIP`/`Build` |
| daemon gRPC server | `pkg/chaosdaemon/server.go` | `BuildServer` |
| tc 注入实现 | `pkg/chaosdaemon/tc_server.go` | `SetTcs` |
| namespace 进入包装 | `pkg/bpm/build_linux.go` | `SetNS` |
| namespace 路径常量 | `pkg/bpm/bpm.go` | `GetNsPath` |
| 展示态计算 | `pkg/status/status.go` | `ChaosStatus` 常量 |
| finished 判断 | `controllers/utils/controller/finished.go` | `IsChaosFinished` |
