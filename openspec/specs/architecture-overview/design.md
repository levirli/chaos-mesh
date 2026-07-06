# Chaos Mesh 整体架构 — 设计参考

本文件是 `openspec/specs/architecture-overview/spec.md` 的配套设计参考。spec 用 Requirement/Scenario 固化架构契约；本文承载契约无法表达的非契约性内容：架构图、数据流、组件通信细节、daemon 注入实现、关键代码 `file:line` 锚点。**本文描述的是现有实现的"现状"，不是待实现的设计。**

## Context

Chaos Mesh 是 CNCF 孵化项目，云原生混沌工程平台，核心目标是让用户通过 CRD 在 Kubernetes 集群中方便地注入各类故障（网络、IO、CPU、内存、时间、JVM、DNS、块设备、云厂商等），并通过 Web UI 设计、监控实验。

其架构严格分层：控制平面做决策，数据平面做执行，两者通过 Kubernetes API 与 gRPC 解耦。

## Goals / Non-Goals

**Goals:**
- 用架构图和数据流图展示控制平面、数据平面、核心组件的关系。
- 说明三种通信方式（K8s API / gRPC / HTTP）分别用在什么地方。
- 列出技术栈及关键依赖版本。
- 深入解释 chaos-daemon 如何把故障“打进”容器（PID 获取、namespace 进入、cgroup attach）。
- 提供关键代码 `file:line` 锚点索引。

**Non-Goals:**
- 不重复 `chaos-experiment-lifecycle` 已覆盖的通用 pipeline、phase 状态机、NetworkChaos 两阶段握手（那里讲得更细）。
- 不覆盖 Workflow 编排、Schedule 调度、webhook 校验、selector 细节。
- 不指导如何修改代码；本文是 reference，不是实现计划。

## Decisions

### D1. 控制平面 vs 数据平面

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         控制平面 (Control Plane)                         │
│  ┌─────────────────────────┐      ┌─────────────────────────────────┐   │
│  │ chaos-controller-manager │     │ chaos-dashboard                  │   │
│  │                         │◀────▶│ (Web UI / REST API)              │   │
│  │ • Workflow Controller   │      │                                  │   │
│  │ • Schedule Controller   │      │ • 可视化实验设计                  │   │
│  │ • 各类 Chaos Controller │      │ • 实验状态监控                    │   │
│  │ • PodNetworkChaos Ctrl  │      │ • OIDC 认证（可选）               │   │
│  │ • Multicluster Ctrl     │      │                                  │   │
│  └───────────┬─────────────┘      └─────────────────────────────────┘   │
│              │                                                           │
│              │  watch / reconcile (controller-runtime client)            │
│              ▼                                                           │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │           Kubernetes API Server + etcd (CRD 权威状态)             │    │
│  │  PodChaos / NetworkChaos / IOChaos / StressChaos / Workflow ...  │    │
│  └───────────────────────────┬─────────────────────────────────────┘    │
│                              │                                           │
└──────────────────────────────┼───────────────────────────────────────────┘
                               │  gRPC (默认 31767)
┌──────────────────────────────┼───────────────────────────────────────────┐
│                              ▼                                           │
│  ┌─────────────────────────────────────────────────────────────────┐     │
│  │                      数据平面 (Data Plane)                        │     │
│  │                   chaos-daemon (DaemonSet)                        │     │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌────────┐ │     │
│  │  │ TC/IPT  │  │ Stress  │  │ IO/JVM  │  │  Time   │  │  DNS   │ │     │
│  │  │ 网络    │  │ CPU/Mem │  │ 时间/IO │  │ 时间偏移 │  │ DNS    │ │     │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘  └────────┘ │     │
│  └─────────────────────────────────────────────────────────────────┘     │
│                               │                                          │
│                               ▼                                          │
│                       目标业务 Pod (Target Pods)                          │
└──────────────────────────────────────────────────────────────────────────┘
```

**控制平面**：chaos-controller-manager 负责“决策”——监听 CRD、编排实验、调度、状态管理；dashboard 提供人机界面。

**数据平面**：chaos-daemon 负责“执行”——以 DaemonSet 运行在节点上，Privileged 权限，进入目标 Pod 的 Namespace/cgroup 实施具体故障。

### D2. 核心组件职责

| 组件 | 入口目录 | 主要职责 | 是否 privileged |
| --- | --- | --- | --- |
| **chaos-controller-manager** | `cmd/chaos-controller-manager/` | 控制核心，包含所有 CRD Controller、Workflow / Schedule 控制器、多集群控制器 | 否 |
| **chaos-daemon** | `cmd/chaos-daemon/` | 数据平面代理，接受 controller gRPC 指令，实际注入故障 | 是 |
| **chaos-daemon-helper** | `cmd/chaos-daemon-helper/` | daemon 的辅助小工具（如 nsexec 场景下的额外逻辑） | 是（随 daemon） |
| **chaos-dashboard** | `cmd/chaos-dashboard/` | Web UI + REST API | 否 |
| **chaos-builder** | `cmd/chaos-builder/` | 代码生成工具，用于生成 CRD 相关脚手架 | 开发时使用 |
| **watchmaker** | `cmd/watchmaker/` | 时间偏移相关辅助程序 | 是 |

controller-manager 内部再分层：

```
controllers/
├── common/              # 通用流水线（所有 chaos 共用）
│   ├── finalizers/
│   ├── desiredphase/
│   ├── condition/
│   ├── records/
│   └── pipeline/
├── chaosimpl/           # 每种 chaos 的具体实现
│   ├── networkchaos/
│   ├── iochaos/
│   ├── stresschaos/
│   └── ...
├── podnetworkchaos/     # 中间 CRD 执行器（调 daemon）
├── podiochaos/          # 中间 CRD 执行器
├── podhttpchaos/        # 中间 CRD 执行器
├── schedule/            # Schedule 控制器
├── multicluster/        # 多集群控制器
└── workflow/            # Workflow 控制器
```

### D3. 三种通信方式

#### 3.1 controller-manager ↔ Kubernetes API

- 使用 `controller-runtime` 的 `client.Client` / `manager.Manager` Watch CRD 资源。
- 所有 chaos 实验以 CRD（`api/v1alpha1`）形式持久化到 etcd。
- controller-runtime 版本：`sigs.k8s.io/controller-runtime v0.21.0`。
- K8s client-go 版本：`k8s.io/client-go v0.35.3`。

#### 3.2 controller-manager ↔ chaos-daemon

- **gRPC**。定义在 `pkg/chaosdaemon/pb/chaosdaemon.proto`。
- controller 根据目标 Pod 所在节点，找到对应 daemon，下发注入/恢复指令。
- 默认端口：`31767`（gRPC）、`31766`（HTTP metrics/pprof）。

#### 3.3 dashboard ↔ 用户 / controller-manager

- **HTTP/REST**：dashboard 使用 `gin-gonic/gin` 提供 API，前端 `ui/` 通过 HTTP 与其交互。
- dashboard 也直接读写 K8s API / CRD 来管理实验。

### D4. 技术栈详情

| 类别 | 技术 | 版本/说明 |
| --- | --- | --- |
| 语言 | Go | `go 1.25.11` |
| K8s Client | `k8s.io/client-go` | `v0.35.3` |
| K8s API / Apimachinery | `k8s.io/api`, `k8s.io/apimachinery` | `v0.35.3` |
| CRD / Controller 框架 | `sigs.k8s.io/controller-runtime` | `v0.21.0` |
| gRPC | `google.golang.org/grpc` | `v1.79.3` |
| 依赖注入 | `go.uber.org/fx` | `v1.19.2` |
| 日志 | `github.com/go-logr/logr` + zap | `v1.4.3` |
| Web 框架 | `gin-gonic/gin` | `v1.10.1` |
| 数据库 | `gorm.io/gorm` + SQLite/MySQL/Postgres/SQLServer | dashboard 元数据 |
| 前端 | React / TypeScript | `ui/` 目录，pnpm 管理 |
| 构建 | Makefile + Docker (build-env / dev-env) | 容器化构建 |
| 测试 | Ginkgo v2 + Gomega | `onsi/ginkgo/v2`, `onsi/gomega` |
| 云 SDK | AWS SDK v2, Azure SDK, GCP API | 用于云厂商 chaos |

### D5. 两种控制器与中间 CRD

对于需要进 Pod namespace 的 chaos（NetworkChaos / IOChaos / HTTPChaos），架构采用**两层控制器 + 中间 CRD**：

```
用户 NetworkChaos CRD
        │
        ▼
┌─────────────────────────────────────┐
│  networkchaos-pipeline controller   │  ← 通用流水线
│  (controllers/common pipeline)      │
└─────────────┬───────────────────────┘
              │ 写意图
              ▼
      PodNetworkChaos 中间 CRD
              │
              ▼ watch
┌─────────────────────────────────────┐
│  podnetworkchaos controller         │  ← 真正调 daemon 的执行器
│  (controllers/podnetworkchaos)      │
└─────────────┬───────────────────────┘
              │ gRPC
              ▼
       chaos-daemon
```

为什么这样做：
- 用户层 CRD 只描述“意图”；
- 中间 CRD 描述“对某个 Pod 要施加的网络/IO 状态”；
- daemon 只负责“执行具体系统调用”。

### D6. controller 如何找到正确的 daemon

`controllers/utils/chaosdaemon/chaosdaemon.go` 里的 `ChaosDaemonClientBuilder`：

1. 拿到目标 Pod 的 `Spec.NodeName`；
2. 查询同一 namespace 下 `chaos-daemon-*` 的 `EndpointSlice`，找到对应节点的 IP；
3. 用 `pkg/grpc/utils.go` 里的 builder 创建 gRPC 连接（支持 TLS / insecure）；
4. 返回 `pkg/chaosdaemon/client` 封装好的 client。

```
targetPod.Spec.NodeName = "node-1"
        │
        ▼
EndpointSlice: chaos-daemon-xxxx
  endpoint.nodeName == "node-1" → 10.0.0.5
        │
        ▼
gRPC dial 10.0.0.5:31767
```

### D7. chaos-daemon gRPC 服务

定义在 `pkg/chaosdaemon/pb/chaosdaemon.proto`：

| RPC | 用途 |
| --- | --- |
| `SetTcs` | 网络延迟、丢包、重排、损坏、带宽限制 |
| `FlushIPSets` | 设置 ipset（用于网络分区目标选择） |
| `SetIptablesChains` | 设置 iptables 规则（分区、丢包等） |
| `SetTimeOffset` / `RecoverTimeOffset` | 时间偏移 |
| `ContainerKill` / `ContainerGetPid` | 杀容器 / 获取 PID |
| `ExecStressors` / `CancelStressors` | CPU / Memory stress |
| `ApplyIOChaos` | IO 故障注入 |
| `ApplyHttpChaos` | HTTP 故障注入 |
| `ApplyBlockChaos` / `RecoverBlockChaos` | 块设备故障 |
| `SetDNSServer` | DNS 故障 |
| `InstallJVMRules` / `UninstallJVMRules` | JVM 注入 |

### D8. daemon 如何找到目标容器 PID

`pkg/chaosdaemon/crclients/client.go` 定义了抽象接口：

```go
type ContainerRuntimeInfoClient interface {
    GetPidFromContainerID(ctx context.Context, containerID string) (uint32, error)
    ContainerKillByContainerID(ctx context.Context, containerID string) error
    FormatContainerID(ctx context.Context, containerID string) (string, error)
    ListContainerIDs(ctx context.Context) ([]string, error)
    GetLabelsFromContainerID(ctx context.Context, containerID string) (map[string]string, error)
    GetSandboxPidFromPodUID(ctx context.Context, podUID string) (uint32, error)
}
```

实现三种容器运行时：

| Runtime | 实现文件 |
| --- | --- |
| Docker | `pkg/chaosdaemon/crclients/docker/client.go` |
| Containerd | `pkg/chaosdaemon/crclients/containerd/client.go` |
| CRI-O | `pkg/chaosdaemon/crclients/crio/client.go` |

以 Containerd 为例：通过 socket 连到 containerd，用 container ID 找到 task，返回 `task.Pid()`。

对于沙箱类容器（如 gVisor、Kata），可能拿不到业务容器 PID，因此 fallback 到 `GetSandboxPidFromPodUID`：根据 Pod UID 找到 pause 容器（sandbox）的 PID，因为 pause 容器一定在跑，且网络 namespace 通常由它持有。

### D9. daemon 如何进入目标 namespace

chaos-daemon 不直接调用 Linux `setns()`，而是包装了一个外部二进制 `nsexec`，从独立 release 下载到镜像的 `/usr/local/bin/nsexec`。

`pkg/bpm/build_linux.go` 的 `Build()` 方法会把原始命令改写成：

```bash
/usr/local/bin/nsexec -n /proc/<pid>/ns/net -- tc qdisc add dev eth0 ...
```

多个 namespace 时：

```bash
/usr/local/bin/nsexec -n /proc/<pid>/ns/net -m /proc/<pid>/ns/mnt -- <cmd>
```

`bpm` 支持进入这些 namespace：

```go
MountNS NsType = "mnt"
IpcNS   NsType = "ipc"
NetNS   NsType = "net"
PidNS   NsType = "pid"
```

不同 chaos 类型进入的 namespace 不同：

| Chaos 类型 | 进入的 namespace | 原因 |
| --- | --- | --- |
| NetworkChaos (tc/iptables/ipset) | `NetNS` | 改容器的网络设备、qdisc、iptables |
| IOChaos | `MountNS` + `PidNS` | 在容器里挂载/替换文件、访问 proc |
| HTTPChaos | `PidNS` + `NetNS` | 把 tproxy 进程挂到容器网络 |
| JVMChaos | `MountNS` + `NetNS` | 装载 Byteman agent |
| DNSChaos | `MountNS` | 改写 `/etc/resolv.conf` |
| StressChaos | `PidNS`（可选）+ cgroup attach | stress-ng 进程本身可以跑在宿主机，但用 cgroup 限制目标容器资源 |

### D10. SetTcs 注入网络故障的完整流程

`pkg/chaosdaemon/tc_server.go` 里的 `SetTcs`：

```
1. 从 gRPC 请求拿到 containerID / podUID
2. pid, err := crClient.GetPidFromContainerID(...)
3. 如果失败，fallback 到 crClient.GetSandboxPidFromPodUID(...)
4. tcCli := buildTcClient(ctx, log, enterNS, pid)
5. 获取容器所有网卡接口
6. flush 旧 tc 规则
7. 按设备分组，逐条添加：
   - globalTc：直接串在 root 下的 netem / tbf
   - filterTc：加 prio qdisc + sfq + iptables CLASSIFY
```

实际执行的命令形如：

```bash
tc qdisc add dev eth0 root handle 1: netem delay 50ms
tc qdisc add dev eth0 parent 1: handle 2: netem delay 100ms
tc qdisc add dev eth0 parent 2: handle 3: prio bands 5 ...
iptables -A TC-TABLES-0 -o eth0 -m set --match-set A dst -j CLASSIFY --set-class 3:4 -w 5
```

所有 `tc` / `iptables` 命令都通过 `bpm` + `nsexec` 进入目标容器的 `NetNS` 执行。

### D11. StressChaos 通过 cgroup 施加资源压力

`pkg/chaosdaemon/stress_server_linux.go` 里的 `ExecCPUStressors`：

```go
pid, err := s.crClient.GetPidFromContainerID(ctx, req.Target)
attachCGroup, err := cgroups.GetAttacherForPID(int(pid))

processBuilder := bpm.DefaultProcessBuilder("stress-ng", ...).EnablePause()
if req.EnterNS {
    processBuilder = processBuilder.SetNS(pid, bpm.PidNS)
}
cmd := processBuilder.Build(ctx)

proc, err := s.backgroundProcessManager.StartProcess(ctx, cmd)
err = attachCGroup.AttachProcess(proc.Pair.Pid)

// 用 SIGCONT 唤醒 pause 进程，真正开始 stress
```

关键点：
- `stress-ng` 启动时被 `pause` 包装，先不运行；
- 启动后把 stress-ng 的 PID attach 到目标容器的 cgroup；
- 然后发 `SIGCONT` 唤醒它开始运行；
- 这样即使 stress-ng 跑在宿主机 PID namespace，也会被目标容器的 CPU/Memory cgroup 限制。

cgroup v1 和 v2 都支持：

- **v1**：`cgroups.Load(V1, path).Add(Process{Pid: pid})`
- **v2**：因为 cgroup namespace 隔离，需要 `nsenter -C -t 1 -- sh -c "echo <pid> >> /host-sys/fs/cgroup<path>/cgroup.procs"`

### D12. pause 包装的作用

`bpm` 里的 `EnablePause()` 会让命令变成：

```bash
/usr/local/bin/pause <original command>
```

`pause` 也是一个预置二进制。它先把进程暂停，等 daemon 完成 cgroup attach、iptables 准备等“附属操作”后再 `SIGCONT` 唤醒。这样可以避免进程在还没被正确限制/配置前就开始运行。

### D13. daemon 的清理机制

`CancelStressors` 会根据之前返回的 `CpuInstanceUid` / `MemoryInstanceUid`，在 `backgroundProcessManager` 里找到对应进程并杀掉。所有后台进程由 `bpm.BackgroundProcessManager` 统一管理，支持通过 UID 或 `Pid + CreateTime` 定位。

### D14. 端到端数据流示例

以用户创建 `NetworkChaos` 为例，完整数据流：

```
用户 apply NetworkChaos
        │
        ▼
Kubernetes API Server (etcd)
        │
        ▼
controller-manager: networkchaos-pipeline
  ├─ desiredphase: 计算 DesiredPhase = Running
  ├─ records: Selector 选出目标 Pod，对每个 Pod 调用 trafficcontrol.Impl.Apply
  │            生成/更新 PodNetworkChaos
        │
        ▼
controller-manager: podnetworkchaos controller (watch 子 CR)
  ├─ 找到目标 Pod
  ├─ 通过 EndpointSlice 定位同节点 chaos-daemon IP
  ├─ gRPC 调用 SetTcs / FlushIPSets / SetIptablesChains
        │
        ▼
chaos-daemon (同节点 DaemonSet)
  ├─ crClient.GetPidFromContainerID(containerID)
  ├─ 失败则 fallback 到 sandbox PID
  ├─ bpm + nsexec 进入目标容器的 NetNS
  └─ 执行 tc / iptables / ipset 等系统调用
        │
        ▼
   目标容器网络栈被改变
```

## Risks / Trade-offs

- **[daemon 需要 privileged 是设计上的必要]**：进入 namespace、修改 cgroup、执行 tc/iptables 都需要高权限。这是数据平面 unavoidable 的权限需求，控制平面则不需要 privileged。
- **[controller 与 daemon 的 gRPC 通信需要保护]**：虽然默认在同一集群内部，但生产环境建议启用 TLS/mTLS 防止未授权访问。
- **[中间 CRD 增加了架构复杂度]**：NetworkChaos / IOChaos / HTTPChaos 需要两层控制器和子 CR，换来了“意图”与“执行”的解耦，以及多实验共存的幂等性。简单 chaos（如 pod-kill）不需要中间 CRD。
- **[nsexec 是外部二进制]**：从 `chaos-mesh/nsexec` release 下载，版本与 daemon 镜像耦合。升级时需要同步更新。
- **[容器运行时支持有限]**：目前只支持 Docker、Containerd、CRI-O。其他运行时（如 containerd 的某些安全沙箱变体）可能需要扩展 `ContainerRuntimeInfoClient`。
- **[本文是现状描述，可能随代码演进过时]** 文中 `file:line` 锚点基于本仓库当前 develop 分支。代码重构后锚点会漂移。

## 关键代码锚点索引

| 机制 | 文件 | 关键位置 |
| --- | --- | --- |
| controller-manager 入口 | `cmd/chaos-controller-manager/main.go` | `main` |
| daemon 入口 | `cmd/chaos-daemon/main.go` | `main` |
| dashboard 入口 | `cmd/chaos-dashboard/main.go` | `main` |
| controller 模块组装 | `controllers/fx.go` | `Module` |
| 通用流水线注册 | `controllers/common/fx.go` | `Bootstrap` |
| pipeline 五步顺序 | `controllers/common/step.go` | `AllSteps` |
| NetworkChaos 子 CR 注册 | `controllers/chaosimpl/networkchaos/impl.go` | `NewImpl` |
| PodNetworkChaos 执行器 | `controllers/podnetworkchaos/controller.go` | `Reconcile` |
| daemon client builder | `controllers/utils/chaosdaemon/chaosdaemon.go` | `FindDaemonIP` / `Build` |
| gRPC proto 定义 | `pkg/chaosdaemon/pb/chaosdaemon.proto` | `service ChaosDaemon` |
| daemon server 构造 | `pkg/chaosdaemon/server.go` | `BuildServer` / `newDaemonServer` |
| tc 注入实现 | `pkg/chaosdaemon/tc_server.go` | `SetTcs` |
| iptables 注入实现 | `pkg/chaosdaemon/iptables_server.go` | `SetIptablesChains` |
| ipset 注入实现 | `pkg/chaosdaemon/ipset_server.go` | `FlushIPSets` |
| stress 注入实现 | `pkg/chaosdaemon/stress_server_linux.go` | `ExecCPUStressors` / `ExecMemoryStressors` |
| IO 注入实现 | `pkg/chaosdaemon/iochaos_server.go` | `ApplyIOChaos` |
| HTTP 注入实现 | `pkg/chaosdaemon/httpchaos_server.go` | `ApplyHttpChaos` |
| 时间注入实现 | `pkg/chaosdaemon/time_server_linux.go` | `SetTimeOffset` |
| DNS 注入实现 | `pkg/chaosdaemon/dns_server.go` | `SetDNSServer` |
| JVM 注入实现 | `pkg/chaosdaemon/jvm_server.go` | `InstallJVMRules` |
| Block 注入实现 | `pkg/chaosdaemon/blockchaos_server_linux.go` | `ApplyBlockChaos` |
| 容器运行时接口 | `pkg/chaosdaemon/crclients/client.go` | `ContainerRuntimeInfoClient` |
| containerd 客户端 | `pkg/chaosdaemon/crclients/containerd/client.go` | `GetPidFromContainerID` |
| docker 客户端 | `pkg/chaosdaemon/crclients/docker/client.go` | `GetPidFromContainerID` |
| cri-o 客户端 | `pkg/chaosdaemon/crclients/crio/client.go` | `GetPidFromContainerID` |
| cgroup attach | `pkg/chaosdaemon/cgroups/attach_cgroup.go` | `AttachCGroupV1` / `AttachCGroupV2` |
| namespace 进入包装 | `pkg/bpm/build_linux.go` | `Build` |
| namespace 路径常量 | `pkg/bpm/bpm.go` | `GetNsPath` / `NsType` |
| 后台进程管理 | `pkg/bpm/bpm.go` | `BackgroundProcessManager` |
| gRPC 连接 builder | `pkg/grpc/utils.go` | `Builder` |
| CRD 类型定义 | `api/v1alpha1/` | 各 chaos types |
| dashboard API | `pkg/dashboard/` | REST API 实现 |
| 前端 UI | `ui/` | React + TypeScript |
| go.mod 依赖 | `go.mod` | 所有依赖版本 |
