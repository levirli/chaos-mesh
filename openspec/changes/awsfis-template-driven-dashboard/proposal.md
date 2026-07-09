## Why

当前 `add-awsfischaos-fault-injection` change（已完成 28/29 任务）采用的是 "CR 驱动 + Apply() 三步合并" 架构：`AWSFISChaos` CR 的 reconciler 在 `Apply()` 里串行执行 `CreateExperimentTemplate → StartExperiment → DeleteExperimentTemplate`，模板被设计为一次性载体（`impl.go:175-179` 注释明确写着"删除模板避免堆积"）。这带来两个问题：

1. 运维人员无法从 AWS FIS 控制台查看、复用或管理实验模板——它们被设计成用完即删。用户刚创建的 "Reboot DB Instances" 模板就是典型症状：dashboard 提示创建成功，FIS 后台却看不到任何东西。
2. CR 驱动模型把"定义一次故障"和"立即运行一次故障"混为一谈。运维人员想"定义一份模板，按需多次启动实验"的需求没有路径可走。

本次 change 转向**模板持存 + dashboard 驱动**的模型：dashboard 直接对接 AWS FIS，承担模板的列表 / 创建 / 删除 / 启动实验 / 列举实验五项操作；`AWSFISChaos` CRD 及其 controller 整体移除。dashboard 成为 FIS 模板生命周期的唯一入口。

本 change **取代（supersedes）** `add-awsfischaos-fault-injection`（28/29 任务已完成但尚未 apply）。旧 change 的 CR 驱动架构被放弃，改用本文档描述的模型。

## What Changes

- **BREAKING**：整体移除 `AWSFISChaos` CRD——包括 types、webhooks、groupversion 信息、deepcopy 生成代码，以及整个 `controllers/chaosimpl/awsfischaos/` 包（含 `actions.go`、`impl.go` 及子目录 `fisutil/`、`rdsreboot/`、`rdsfailover/`、`elasticacheinterruptaz/`、`dynamodbpausereplication/`）。
- **BREAKING**：移除 CRD manifest `helm/chaos-mesh/crds/chaos-mesh.org_awsfischaos.yaml`，以及 helm chart 中与 awsfischaos 相关的 RBAC / controller 注册配置。
- 新增 dashboard FIS API 服务，位于 `pkg/dashboard/apiserver/aws/` 下新建的 `fis/` 子包，对外暴露五个端点，分别代理 AWS FIS：
  - `GET    /aws/fis/templates`           → `fis:ListExperimentTemplates`
  - `POST   /aws/fis/templates`           → `fis:CreateExperimentTemplate`
  - `DELETE /aws/fis/templates/:id`       → `fis:DeleteExperimentTemplate`
  - `POST   /aws/fis/templates/:id/start` → `fis:StartExperiment`
  - `GET    /aws/fis/experiments`         → `fis:ListExperiments`
- 把 `actionTable`（actionId → resourceType / targetKey / selection / actionParameters / targetParameters）以及 `goDurationToISO8601` 辅助函数从即将被删的 controller 包迁移到新的 dashboard FIS 服务包——controller 删除后，dashboard 创建模板时是这段逻辑的唯一调用方。
- AWS IAM：dashboard 用的 AWS 凭证（K8s Secret 里的 access key / secret key / session token）对应的 IAM principal 需要授权 `fis:ListExperimentTemplates` / `fis:CreateExperimentTemplate` / `fis:DeleteExperimentTemplate` / `fis:StartExperiment` / `fis:ListExperiments`（以及现有的 RDS 描述权限）。本期**不**需要 `fis:StopExperiment`。权限要求写入 `helm/chaos-mesh/README.md`。**注意：这是 AWS IAM 权限，不是 K8s RBAC**——dashboard 调用 AWS FIS 走 AWS SDK HTTP 签名，不经过 K8s kube-apiserver。
- 前端将 FIS 菜单入口指向新的**模板列表页** `/fis`，页面顶部放两个按钮（`New Experiment Template`、`Experiment Info`），每行模板右侧带 `Start` / `Delete` 两个操作按钮。现有的 action-card + 表单页移到 `/fis/new`，提交目标从"创建 AWSFISChaos CR"改为"调用 `POST /aws/fis/templates`"。新增 `/fis/experiments` 页面，从 `GET /aws/fis/experiments` 拉取实验列表。
- 保留 `pkg/config/...AWSFISConfig`（`Region` / `RoleArn` / `SecretName` / `SecretNamespace` / `ReportS3Bucket`）——dashboard 仍需读取它来构造 FIS client 和填充 `CreateExperimentTemplateInput.RoleArn`。只清理 controller 侧的引用。

### 已知限制（本期不解决）

1. ElastiCache `interrupt-az-power` 是可停止型故障，但本期没有 CR `Recover()` 钩子，也没有 `Stop` 按钮，运维人员一旦启动就无法从 dashboard 停止。后续 change 会补 `fis:StopExperiment` 端点和 `/aws/fis/experiments` 页面的 `Stop` 按钮。
2. 集群中已存在的 `AWSFISChaos` CR 实例在 CRD 被删除后会变成孤儿。运维人员需在升级前/升级中手动清理。具体步骤写入 design.md 的迁移计划。
3. controller 侧对 FIS 的可观测性（events、metrics）随之消失。如果未来需要把实验状态回写到 Chaos Mesh，另开 change 处理。

## Capabilities

### New Capabilities
- `awsfis-template-management`：Dashboard 驱动的 AWS FIS 实验模板生命周期管理——列表、创建、删除、从模板启动实验，以及列举运行中/已完成的实验。取代原 CR 驱动的注入流程。

### Modified Capabilities
<!-- 无。旧 change 没有在 openspec/specs/ 下沉淀过 spec，因此没有现存的 spec 级 requirement 需要 delta。 -->

## Impact

- **删除的代码**：`api/v1alpha1/awsfischaos_*`、整个 `controllers/chaosimpl/awsfischaos/` 包、awsfischaos controller 注册入口、`helm/chaos-mesh/crds/chaos-mesh.org_awsfischaos.yaml`。`controllers/chaosimpl/awsfischaos/actions.go` 是迁移而非删除（见上文）。
- **新增的代码**：`pkg/dashboard/apiserver/aws/fis/` 子包，包含五个端点的 handler、迁移后的 `actionTable`、请求/响应模型。`pkg/dashboard/swaggerdocs/` 下新增对应 swagger 条目。
- **前端**：`ui/app/src/pages/FIS/` 重构（列表页 + 新建页 + 实验页三页）。router、sidebar 菜单、i18n（`zh.json` / `en.json`）相应更新。
- **Helm chart**：dashboard 的 K8s `Role` / `RoleBinding` 不变（dashboard 调用 AWS FIS 走 AWS SDK，不经 K8s RBAC）；CRD manifest 移除；移除 chaos-controller-manager 上与 awsfischaos 相关的 RBAC。AWS IAM 权限要求写入 `helm/chaos-mesh/README.md`。
- **配置**：`AWSFISConfig` 保留，环境变量 `AWSFIS_ROLE_ARN` / `AWSFIS_REGION` / `AWSFIS_SECRET_NAME` / `AWSFIS_SECRET_NAMESPACE` / `AWSFIS_REPORT_S3_BUCKET` 语义不变。
- **Supersedes**：`openspec/changes/add-awsfischaos-fault-injection/` 保留在磁盘上作为历史，不再是 AWS FIS 行为的事实来源。
- **运维影响**：已部署 `AWSFISChaos` CR 的集群升级前需手动清理 CR；CRD 删除对这类安装是 breaking change。
