yui# Chaos Mesh 整体架构规格

## Purpose

固化 Chaos Mesh 项目的整体架构、核心组件、组件间通信方式与技术栈，作为新成员切入代码与后续设计的 reference 规范。

Chaos Mesh 是一个云原生混沌工程平台，运行在 Kubernetes 之上，通过 CRD 描述实验、通过控制器编排实验、通过节点级 DaemonSet 执行实际故障注入。

## Requirements

### Requirement: 架构分为控制平面与数据平面

系统 SHALL 明确划分为控制平面（Control Plane）与数据平面（Data Plane），两者职责不可混淆。

#### Scenario: 控制平面负责决策与编排
- **WHEN** 用户创建/修改/删除 Chaos 实验
- **THEN** 控制平面负责把用户意图持久化为 CRD、计算期望状态、选择目标、调度注入/恢复，但不直接操作目标容器

#### Scenario: 数据平面负责实际注入
- **WHEN** 控制平面决定需要对某个 Pod 注入故障
- **THEN** 数据平面进入该 Pod 的 namespace/cgroup 执行具体系统调用（tc/iptables/stress-ng 等）

### Requirement: 核心组件及职责

系统 SHALL 由以下核心组件组成：

| 组件 | 类型 | 主要职责 |
| --- | --- | --- |
| chaos-controller-manager | Deployment | 控制核心，包含所有 CRD Controller、Workflow/Schedule 控制器、多集群控制器 |
| chaos-daemon | DaemonSet | 节点级代理，执行网络/IO/Stress/Time/JVM/DNS/Block/ContainerKill 等实际注入 |
| chaos-dashboard | Deployment | Web UI + REST API，供用户设计、提交、监控实验 |
| chaos-daemon-helper | 辅助二进制 | daemon 的辅助小工具（如 nsexec 场景下的额外逻辑） |
| chaos-builder | 代码生成工具 | 生成 CRD 相关脚手架 |
| watchmaker | 辅助二进制 | 时间偏移相关辅助程序 |

#### Scenario: controller-manager 包含通用流水线
- **WHEN** 任意 Chaos CR 被创建
- **THEN** controller-manager 为其注册一个 pipeline controller，按固定五步执行：finalizers init → desiredphase → condition → records → finalizers clean

#### Scenario: daemon 以特权运行
- **WHEN** daemon Pod 启动
- **THEN** 默认以 privileged 权限运行，能够 remount /sys、访问容器运行时 socket、进入目标 Pod namespace

### Requirement: 组件间通信方式

组件间 SHALL 只通过以下三种方式通信：

| 通信双方 | 协议 | 用途 |
| --- | --- | --- |
| controller-manager ↔ Kubernetes API | HTTP/HTTPS (controller-runtime client) | Watch CRD、更新 status、操作子资源 |
| controller-manager ↔ chaos-daemon | gRPC (默认端口 31767) | 下发注入/恢复指令 |
| dashboard ↔ 用户 / controller-manager | HTTP/REST (gin-gonic/gin) | Web UI 与 API 交互；dashboard 也直接访问 K8s API |

#### Scenario: controller 通过 K8s API 协调状态
- **WHEN** 用户创建 NetworkChaos
- **THEN** controller-manager 不直接调 daemon，而是先创建/更新 PodNetworkChaos 子 CR，再由子 CR 控制器调 daemon

#### Scenario: controller 通过 gRPC 找到正确 daemon
- **WHEN** 需要对某 Pod 注入故障
- **THEN** controller SHALL 根据 `targetPod.Spec.NodeName` 匹配同节点 chaos-daemon 的 EndpointSlice，获取 daemon IP 后建立 gRPC 连接

#### Scenario: dashboard 提供人机界面
- **WHEN** 用户在 Web UI 设计实验
- **THEN** dashboard 通过 gin REST API 接收请求，再转换为对 K8s API 的 CRD 操作

### Requirement: 技术栈版本

项目 SHALL 使用以下核心技术栈：

| 类别 | 技术 | 版本 |
| --- | --- | --- |
| 语言 | Go | 1.25.11 |
| K8s Client | k8s.io/client-go | v0.35.3 |
| K8s API/Apimachinery | k8s.io/api, k8s.io/apimachinery | v0.35.3 |
| CRD/Controller 框架 | sigs.k8s.io/controller-runtime | v0.21.0 |
| gRPC | google.golang.org/grpc | v1.79.3 |
| 依赖注入 | go.uber.org/fx | v1.19.2 |
| 日志 | github.com/go-logr/logr + zap | v1.4.3 |
| Web 框架 | gin-gonic/gin | v1.10.1 |
| ORM | gorm.io/gorm | v1.31.1 |
| 测试框架 | onsi/ginkgo/v2, onsi/gomega | v2.27.2, v1.38.2 |
| 前端 | React / TypeScript | 见 `ui/` |

#### Scenario: 使用 controller-runtime 管理控制器生命周期
- **WHEN** controller-manager 启动
- **THEN** 使用 `ctrl.NewManager` 创建 manager，所有 controller 通过 `builder.For(...).Complete(...)` 注册到 manager

#### Scenario: 使用 fx 进行依赖注入
- **WHEN** 各模块需要共享 client/manager/logger/selector 等依赖
- **THEN** 使用 `go.uber.org/fx` 的 `fx.Provide`/`fx.In` 机制组装，而非手动 new

### Requirement: CRD 是唯一的权威状态源

所有 chaos 实验 SHALL 以 Kubernetes CustomResourceDefinition（CRD）形式定义，etcd 是持久化权威，控制器通过 Watch 机制响应变化。

#### Scenario: 支持的 chaos 类型
- **WHEN** 用户需要注入某种故障
- **THEN** 可以选择以下 CRD 之一：PodChaos、NetworkChaos、IOChaos、TimeChaos、StressChaos、HTTPChaos、DNSChaos、JVMChaos、KernelChaos、BlockChaos、AWSChaos、GCPChaos、AzureChaos、PhysicalMachineChaos、Workflow 等

#### Scenario: 中间 CRD 解耦意图与执行
- **WHEN** NetworkChaos/IOChaos/HTTPChaos 需要进 Pod namespace
- **THEN** 控制器 SHALL 生成 PodNetworkChaos/PodIOChaos/PodHttpChaos 中间 CRD，由专门 controller watch 并调 daemon 执行

### Requirement: chaos-daemon 支持多种容器运行时

chaos-daemon SHALL 通过容器运行时接口获取目标容器信息，支持 Docker、Containerd、CRI-O。

#### Scenario: 根据容器 ID 获取 PID
- **WHEN** daemon 收到 gRPC 请求
- **THEN** 调用 `ContainerRuntimeInfoClient.GetPidFromContainerID(containerID)` 获取容器主进程 PID

#### Scenario: 沙箱类容器 fallback
- **WHEN** 无法从容器 ID 获取 PID（如某些沙箱场景）
- **THEN** fallback 到 `GetSandboxPidFromPodUID(podUID)`，使用 pause 容器 PID（其网络 namespace 通常被业务容器共享）

### Requirement: chaos-daemon 通过 namespace/cgroup 注入故障

chaos-daemon SHALL 使用 `nsexec` 进入目标 namespace，或将进程 attach 到目标 cgroup 来施加故障。

#### Scenario: 进入网络 namespace 执行 tc/iptables
- **WHEN** 收到 `SetTcs` / `FlushIPSets` / `SetIptablesChains` 请求
- **THEN** daemon 构造命令 `/usr/local/bin/nsexec -n /proc/<pid>/ns/net -- <tc/iptables/ipset>` 在目标 netns 内执行

#### Scenario: stress 通过 cgroup 限制资源
- **WHEN** 收到 `ExecStressors` 请求
- **THEN** daemon 启动 `stress-ng`/`memStress` 进程，然后将其 PID attach 到目标容器的 cgroup，使其受目标容器的 CPU/Memory 限制

### Requirement: 安全与权限隔离

控制平面与数据平面 SHALL 保持权限最小化原则：controller-manager 不需要 privileged；chaos-daemon 需要 privileged 以执行 namespace/cgroup/系统调用。

#### Scenario: controller 不进入 Pod
- **WHEN** 需要注入故障
- **THEN** controller SHALL 只生成 CRD 或发 gRPC，自身不进入目标 Pod namespace

#### Scenario: daemon 只接受 controller 请求
- **WHEN** daemon 暴露 gRPC 服务
- **THEN** 可通过 TLS/mTLS 进行访问控制（可选）
