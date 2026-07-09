## ADDED Requirements

### Requirement: 列举 FIS 实验模板

dashboard SHALL 暴露 `GET /aws/fis/templates`，使用部署级 `AWSFISConfig`（region + credentials）代理调用 AWS FIS `ListExperimentTemplates`。响应 SHALL 为 JSON 数组，每项是模板摘要，至少包含 `id`、`description`、`creationTime`、`tags`。端点 SHALL 在响应前分页拉取完所有 AWS 侧页面。

> 注：AWS `ListExperimentTemplates` 返回的 `ExperimentTemplateSummary` 只含 `id` / `description` / `creationTime` / `tags`，**不返回 `actions` 或 `targets` map**。因此列表页 SHALL NOT 展示故障 action 或 target 列——这些需要对单个模板调用 `GetExperimentTemplate` 才能拿到，超出本期范围。

#### Scenario: AWS 账号下存在模板
- **WHEN** 运维进入 FIS 模板列表页（`/fis`）
- **THEN** dashboard 调用 `GET /aws/fis/templates`，页面渲染一张表，每行对应一个模板，展示 ID、description、creationTime 和一组行操作按钮（`Start` / `Delete`）

#### Scenario: AWS 账号下没有模板
- **WHEN** `GET /aws/fis/templates` 返回空数组
- **THEN** 页面渲染空态消息（"暂无实验模板，点击 New Experiment Template 创建一个"）

#### Scenario: AWS FIS 返回错误
- **WHEN** 上游 `ListExperimentTemplates` 调用失败（认证、限流、region 配错）
- **THEN** 端点响应 HTTP 500，JSON body 的 `message` 字段描述 AWS 错误；页面内联渲染错误

### Requirement: 创建 FIS 实验模板

dashboard SHALL 暴露 `POST /aws/fis/templates`，构造并提交 AWS FIS `CreateExperimentTemplate` 请求。请求体 SHALL 接受 `action`、`description`、`name`、`targetArn`、`forceFailover`、`resourceTags`、`availabilityZoneIdentifier`、`duration`。`name` SHALL 写入模板的 `Name` tag(为空时回退为随机 `chaos-mesh/<uuid8>`);因 `UpdateExperimentTemplate` 无法改 tag,`name` 只能在创建时设置,更新页 SHALL 将其置灰只读。新建模板页标题 SHALL 为 `New FIS Experiment Template`,`Name` 字段 SHALL 位于 `Description` 下方。dashboard SHALL 用迁移后的 `actionTable` 把 `action` 翻译为 FIS 的 `actionId`、`resourceType`、`targetKey`、目标选择模式、action 参数和 target 参数。dashboard SHALL 每次请求生成一个 UUID 作为 `ClientToken`。`AWSFISConfig` 中的 `RoleArn` SHALL 写入 `CreateExperimentTemplateInput.RoleArn`。`StopConditions` SHALL 为 `[{source: "none"}]`，`AccountTargeting` SHALL 为 `single-account`。成功时端点 SHALL 响应所创建模板的 `id`，并把 UI 重定向回 `/fis`。

#### Scenario: 创建一个 RDS reboot action 的模板
- **WHEN** 运维提交新建模板表单，`action = aws:rds:reboot-db-instances`、有效 `targetArn`、`forceFailover = true`
- **THEN** dashboard 构造 `CreateExperimentTemplateInput`，其中 `actions["chaos-action"].actionId = "aws:rds:reboot-db-instances"`、`actions["chaos-action"].parameters.forceFailover = "true"`、`targets["chaos-target"].resourceType = "aws:rds:db"`、`targets["chaos-target"].resourceArns = [<targetArn>]`、`targets["chaos-target"].selectionMode = "ALL"` 并提交；新模板出现在 `GET /aws/fis/templates`

#### Scenario: 创建一个 ElastiCache interrupt-az-power action 的模板
- **WHEN** 运维提交 `action = aws:elasticache:replicationgroup-interrupt-az-power`、`resourceTags`、`availabilityZoneIdentifier`、`duration = "5m"`
- **THEN** dashboard 用 `goDurationToISO8601` 把 `duration` 转为 `PT5M`,填充 `targets["chaos-target"].parameters.availabilityZoneIdentifier`,提交;新模板出现在列表(ElastiCache 目标 SHALL 按 **tag** 选择——AWS FIS 强制要求 `aws:elasticache:replicationgroup` 用 tag,不支持 ARN,也不接受仅用 filter)

#### Scenario: 创建一个 DynamoDB global-table-pause-replication action 的模板
- **WHEN** 运维提交 `action = aws:dynamodb:global-table-pause-replication`、从下拉选中的 `targetArn`(DynamoDB global table ARN)、`duration = "5m"`
- **THEN** dashboard 构造 `targets["chaos-target"].resourceType = "aws:dynamodb:global-table"`、`resourceArns = [<targetArn>]`、`actions["chaos-action"].parameters.duration = "PT5M"`,提交;新模板出现在列表(DynamoDB global-table **支持按 ARN 选择**,已实测确认;ARN 下拉由 `GET /aws/dynamodb/global-tables` 提供,仅列 `replicas>0` 的 global table)
- **AND** 该 action 仅适用于 DynamoDB **global table**(多区域、含副本);普通单区域表不是合法目标

#### Scenario: 缺少必填字段
- **WHEN** 运维提交 RDS action 但未填 `targetArn`
- **THEN** dashboard 响应 HTTP 400 并在 message 中指明缺失字段；不发起任何 AWS API 调用

#### Scenario: AWS 拒绝创建（例如 RoleArn 非法）
- **WHEN** `CreateExperimentTemplate` 返回错误
- **THEN** dashboard 响应 HTTP 500 并返回 AWS 错误消息；未创建任何模板

### Requirement: 删除 FIS 实验模板

dashboard SHALL 暴露 `DELETE /aws/fis/templates/:id`，以 path 参数 `id` 调用 AWS FIS `DeleteExperimentTemplate`。UI SHALL 在提交前弹出二次确认对话框。成功时端点 SHALL 响应 HTTP 204。

#### Scenario: 删除一个已存在的模板
- **WHEN** 运维在某行点击 `Delete` 并确认
- **THEN** dashboard 调用 `DELETE /aws/fis/templates/<id>`，收到 204 后从表中移除该行

#### Scenario: 删除一个不存在的模板
- **WHEN** 运维删除一个已被外部（AWS 控制台等）删除的模板
- **THEN** AWS 返回 `ResourceNotFoundException`；dashboard 响应 HTTP 404，UI 显示错误并刷新列表

### Requirement: 从模板启动实验

dashboard SHALL 暴露 `POST /aws/fis/templates/:id/start`，以 path 参数模板 id 调用 AWS FIS `StartExperiment`。dashboard SHALL 每次请求生成一个 UUID 作为 `ClientToken`。dashboard SHALL 给启动的实验打 tag：`Name = "chaos-mesh/<template-id>-<clienttoken 前缀>"`、`chaos-mesh/template-id = <template-id>`，以便把实验关联回模板。成功时端点 SHALL 响应新实验的 `id`，UI SHALL 显示成功 toast 并跳转到 `/fis/experiments`。

#### Scenario: 启动一个有效模板
- **WHEN** 运维在某行点击 `Start`
- **THEN** dashboard 调用 `POST /aws/fis/templates/<id>/start`，AWS FIS 返回新 experiment id，跳转后实验页显示该实验处于 `pending` / `initiating` / `running` 状态

#### Scenario: 启动一个已不存在的模板
- **WHEN** 运维在某行点击 `Start`，但该模板已被外部删除
- **THEN** AWS 返回 `ResourceNotFoundException`；dashboard 响应 HTTP 404，UI 显示错误并刷新模板列表

### Requirement: 列举 FIS 实验

dashboard SHALL 暴露 `GET /aws/fis/experiments`，代理调用 AWS FIS `ListExperiments`。响应 SHALL 为 JSON 数组，每项是实验摘要，至少包含 `id`、`templateId`、`state.status`、`creationTime`、`tags`。端点 SHALL 在响应前分页拉取完所有 AWS 侧页面。UI SHALL 在 `/fis/experiments` 渲染列表，列至少包括实验 ID、模板 ID（从 `chaos-mesh/template-id` tag 提取，若有）、状态、创建时间。处于非终态（`pending` / `initiating` / `running`）的实验行 SHALL 在操作列显示 `Stop` 按钮（见「停止实验」）。

#### Scenario: 存在实验
- **WHEN** 运维在 `/fis` 点击 `Experiment Info` 按钮
- **THEN** dashboard 跳转到 `/fis/experiments`，调用 `GET /aws/fis/experiments`，页面每行渲染一个实验

#### Scenario: 没有实验
- **WHEN** `GET /aws/fis/experiments` 返回空数组
- **THEN** 页面渲染空态消息

### Requirement: 取单个模板详情

dashboard SHALL 暴露 `GET /aws/fis/templates/:id`,以 path 参数 id 调用 AWS FIS `GetExperimentTemplate`,响应 SHALL 包含模板的 `id`、`description`、`action`(从 actions map 派生)、`resourceType`、`selectionMode`、`targetArn` 或 `resourceTags`、action/target 参数、`roleArn`、`tags`、`creationTime`、`lastUpdateTime`。与列表端点不同,`GetExperimentTemplate` 返回完整 actions/targets map,因此详情页 SHALL 展示 action 与 target。`ResourceNotFoundException` SHALL 映射为 HTTP 404。

#### Scenario: 查看一个模板详情
- **WHEN** 运维在 `/fis` 列表点击某模板的 ID
- **THEN** dashboard 跳转到 `/fis/templates/<id>`,调用 `GET /aws/fis/templates/<id>`,页面展示该模板的 action、target、参数、tags、role、时间

#### Scenario: 模板已不存在
- **WHEN** 该模板已被外部删除
- **THEN** AWS 返回 `ResourceNotFoundException`;dashboard 响应 HTTP 404,详情页显示错误

### Requirement: 更新模板

dashboard SHALL 暴露 `PUT /aws/fis/templates/:id`,接受与创建相同结构的请求体(action + description + target/参数),用迁移后的 `actionTable` 重建 `Actions`/`Targets` map 后调用 AWS FIS `UpdateExperimentTemplate`。action 类型 SHALL 固定(前端只读展示),仅 description 与 target/参数可改。请求的 `action` 不在 `actionTable` 内时端点 SHALL 响应 HTTP 400。更新 SHALL NOT 传 `RoleArn`/`StopConditions`(保留原值)。成功时端点 SHALL 响应更新后的模板详情。`ResourceNotFoundException` SHALL 映射为 HTTP 404。

#### Scenario: 更新一个模板的 description 与 target
- **WHEN** 运维在模板详情页修改 description 和 target ARN 后点击 `更新`
- **THEN** dashboard 调用 `PUT /aws/fis/templates/<id>`,AWS 返回更新后的模板;详情页刷新为新值并显示成功提示

#### Scenario: 模板 action 不受 dashboard 管理
- **WHEN** 模板的 action 不在 dashboard 支持的三种 action 内
- **THEN** 详情页只读展示,不提供 `更新`(后端若收到未知 action 亦返回 HTTP 400)

### Requirement: 取单个实验详情

dashboard SHALL 暴露 `GET /aws/fis/experiments/:id`,以 path 参数 id 调用 AWS FIS `GetExperiment`,响应 SHALL 包含实验的 `id`、`templateId`、`state`、`stateReason`、`error`、`action`、target 信息、参数、`creationTime`、`startTime`、`endTime`、`tags`。`ResourceNotFoundException` SHALL 映射为 HTTP 404。

#### Scenario: 查看一个实验详情
- **WHEN** 运维在 `/fis/experiments` 列表点击某实验的 Experiment ID
- **THEN** dashboard 跳转到 `/fis/experiments/<id>`,调用 `GET /aws/fis/experiments/<id>`,页面展示该实验的 state、action、target、时间等;点击 `关闭` 返回 `/fis/experiments`

### Requirement: 停止实验

dashboard SHALL 暴露 `POST /aws/fis/experiments/:id/stop`,以 path 参数 id 调用 AWS FIS `StopExperiment`。成功时端点 SHALL 响应被停止实验的 `id`。`ResourceNotFoundException` SHALL 映射为 HTTP 404。UI 的实验列表 SHALL 仅对处于非终态(`pending` / `initiating` / `running`)的实验行显示 `Stop` 按钮;点击 SHALL 弹出二次确认,确认后调用停止端点并刷新列表。dashboard 调用凭证对应的 IAM principal SHALL 具备 `fis:StopExperiment`。

> 注:对 one-shot 型故障(RDS reboot/failover)停止无实际意义(瞬时完成,基本不会停留在运行态);`Stop` 主要服务可停止型故障(ElastiCache `interrupt-az-power`)。列表按状态门控而非按 action 类型,因为 `ListExperiments` 不返回 action。

#### Scenario: 停止一个运行中的实验
- **WHEN** 运维在实验列表某个 `running` 行点击 `Stop` 并确认
- **THEN** dashboard 调用 `POST /aws/fis/experiments/<id>/stop`,AWS 返回被停止实验;列表刷新后该实验进入 `stopping` / `stopped` 状态

#### Scenario: 终态实验不显示 Stop
- **WHEN** 实验处于 `completed` / `stopped` / `failed` 等终态
- **THEN** 该行 SHALL NOT 显示 `Stop` 按钮

### Requirement: 模板列表与实验列表互跳

`/fis`(模板列表)SHALL 提供一个跳到 `/fis/experiments` 的 `Experiment` 按钮(取代旧的 `Experiment Info` 文案);`/fis/experiments`(实验列表)SHALL 提供一个跳回 `/fis` 的 `Experiment Templates` 按钮。两个列表的 ID 列 SHALL 可点击进入对应详情页。

#### Scenario: 在两个列表间切换
- **WHEN** 运维在 `/fis` 点击 `Experiment` 按钮
- **THEN** 跳转到 `/fis/experiments`;在该页点击 `Experiment Templates` 按钮 SHALL 跳回 `/fis`

### Requirement: 侧边栏 FIS 菜单入口指向模板列表

dashboard 侧边栏 "AWS FIS" 入口 SHALL 路由到 `/fis`（模板列表页），不再指向旧的新建实验表单。新建实验表单 SHALL 只能通过 `/fis` 上的 `New Experiment Template` 按钮到达。

#### Scenario: 运维点击侧边栏 AWS FIS
- **WHEN** 运维点击侧边栏 "AWS FIS" 项
- **THEN** dashboard 跳转到 `/fis`，渲染模板列表页

### Requirement: AWSFISChaos CRD 被移除

仓库内 SHALL NOT 存在 `api/v1alpha1/awsfischaos_*` types、webhooks、groupversion 文件或 deepcopy 生成代码。`controllers/chaosimpl/awsfischaos/` 包 SHALL NOT 存在。helm chart SHALL NOT 携带 `chaos-mesh.org_awsfischaos` CRD manifest。chaos-controller-manager SHALL NOT 注册 awsfischaos reconciler。`pkg/config` 中的 `AWSFISConfig` 结构体 SHALL 保留，因为 dashboard 仍消费它。

#### Scenario: 变更后的仓库
- **WHEN** 开发者在仓库中搜索 `AWSFISChaos` 类型或 awsfischaos controller 包
- **THEN** `api/v1alpha1/` 或 `controllers/chaosimpl/awsfischaos/` 下没有任何文件定义该 CRD 或其 reconciler；`AWSFISConfig` 的引用只出现在 `pkg/config/` 和 `pkg/dashboard/`

#### Scenario: 变更后的 helm install
- **WHEN** 运维执行 `helm install chaos-mesh chaos-mesh/chaos-mesh`
- **THEN** 未应用任何 `chaos-mesh.org_awsfischaos` CRD；chaos-controller-manager pod 启动时不带任何 FIS 相关 controller goroutine；chaos-dashboard pod 的 K8s `Role` / `ClusterRole` 不含任何 `fis:*` 条目（`fis:*` 是 AWS IAM 权限，见「AWS IAM 凭证包含 FIS 权限」，不经 K8s RBAC）

### Requirement: AWS IAM 凭证包含 FIS 权限

dashboard 调用 AWS FIS 用的 AWS 凭证（来自 `AWSFIS_SECRET_NAME` 指向的 K8s Secret，或 SDK 默认 credential chain）对应的 IAM principal SHALL 被授权以下 AWS IAM actions：`fis:ListExperimentTemplates`、`fis:CreateExperimentTemplate`、`fis:GetExperimentTemplate`、`fis:UpdateExperimentTemplate`、`fis:DeleteExperimentTemplate`、`fis:StartExperiment`、`fis:StopExperiment`、`fis:ListExperiments`、`fis:GetExperiment`。此为 AWS IAM 权限（非 K8s RBAC），由运维在 AWS 账号中配置；helm chart 中 dashboard 的 K8s `Role` / `ClusterRole` 不变。权限要求 SHALL 在 `helm/chaos-mesh/README.md` 中写明。

#### Scenario: dashboard 凭证已授权
- **WHEN** 运维按 README 给 dashboard 用的 IAM principal 授权上述五项 `fis:*` actions
- **THEN** dashboard 的五个 FIS 端点都能成功调用 AWS FIS（假设其他前置条件满足）

#### Scenario: dashboard 凭证未授权
- **WHEN** IAM principal 缺少某项 `fis:*` 权限
- **THEN** 对应端点返回 HTTP 500，错误消息包含 AWS 的 `AccessDenied` 信息

### Requirement: actionTable 迁移到 dashboard 服务

`actionTable`、`targetSelection` 常量、`actionMeta` 结构体、`goDurationToISO8601` 辅助函数 SHALL 位于 `pkg/dashboard/apiserver/aws/fis/`。映射表（actionId → resourceType / targetKey / selection / actionParameters / targetParameters）SHALL 与迁移时点的 controller 侧表逐字等价。迁移后的 `actions_test.go` SHALL 继续在新的包下覆盖该表。

#### Scenario: dashboard 服务使用迁移后的表
- **WHEN** dashboard 的 `POST /aws/fis/templates` handler 构造 `CreateExperimentTemplateInput`
- **THEN** 它从 `pkg/dashboard/apiserver/aws/fis/actions.go` 查 `actionTable`，不 import 任何 controller 包

#### Scenario: controller 包已不存在
- **WHEN** 开发者尝试 import `github.com/chaos-mesh/chaos-mesh/controllers/chaosimpl/awsfischaos`
- **THEN** import 解析到不存在的包；build 快速失败
