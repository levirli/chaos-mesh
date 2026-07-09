## Context

`add-awsfischaos-fault-injection`（本 change 取代的旧 change）当时落地了 CR 驱动模型：`AWSFISChaos` CR 触发一个 reconciler，其 `Apply()` 串行执行 `CreateExperimentTemplate → StartExperiment → DeleteExperimentTemplate`，模板故意设计为一次性载体。运维人员无法从 AWS 控制台管理这些模板——它们被设计成用完即删——而 CR 模型把"定义故障"和"立刻运行故障"两件事耦合在一起。

本次 change 转向 **dashboard 驱动、模板持存** 的 FIS 管理模型。dashboard 通过五个新的 HTTP 端点直接对接 AWS FIS。CRD、controller、webhooks、CRD manifest 全部移除。controller 包里唯一有价值的 `actionTable` 迁移到 dashboard 服务包。

利益相关方：通过 Chaos Mesh 运行 AWS FIS 实验的 SRE / 混沌工程师；升级现有集群的平台运维人员。

关键约束：
- `AWSFISConfig`（Region / RoleArn / SecretName / SecretNamespace / ReportS3Bucket）是部署级固定配置，保持不变。dashboard 已经在 RDS 列举端点（`pkg/dashboard/apiserver/aws/aws.go`）里读取它。
- dashboard 的 FIS client 凭证来源与 RDS 列举一致：同一个 K8s Secret（`aws_access_key_id` / `aws_secret_access_key` / `aws_session_token`）。
- AWS FIS SDK（`github.com/aws/aws-sdk-go-v2/service/fis`）当前通过 controller 间接引入；controller 删除后必须把它提升为 dashboard 的直接依赖。

## Goals / Non-Goals

**Goals:**
- 运维人员可以从 Chaos Mesh dashboard 列举、创建、删除 FIS 实验模板，模板在 AWS FIS 中持久保存，可复用、可巡检。
- 运维人员可一键从某个模板启动实验。
- 运维人员可从 dashboard 查看运行中和已完成的实验。
- `AWSFISChaos` CRD 及其 controller 彻底消失——没有死代码路径，没有孤儿 RBAC。
- `actionTable`（actionId → FIS 模板/目标参数映射）继续驱动模板构造，只是改在 dashboard 服务里调用。

**Non-Goals:**
- 从 dashboard 停止 / 取消一个运行中的实验（`fis:StopExperiment` 端点和 `Stop` 按钮）。已记为后续 change。
- 把 FIS 实验状态作为 Chaos Mesh 状态条件回写（events、metrics）。本期不做；如未来需要，另开 change。
- 多区域 FIS 支持——dashboard 仍然只用部署级那一个 `AWSFIS_REGION`。
- dashboard 内部对"谁能创建/删除模板"的细粒度授权。沿用 dashboard 现有的认证层；不新增按动作的授权。
- 保持 `AWSFISChaos` CR 向后兼容的过渡 shim。CRD 直接删除；现有 CR 升级前必须清理。

## Decisions

### D1. dashboard 作为 AWS FIS 客户端（无 CR / 无 controller）

**决策**：dashboard 服务直接调用 AWS FIS SDK。没有 CR，也没有 reconciler。

**理由**：CR 模型只带来两样东西——(a) K8s 原生意图声明；(b) CR 删除时触发 `Recover()` 停止实验的钩子。(a) 对 AWS FIS 没价值，因为 dashboard 本身就是天然的模板编辑入口，模板本来就存 AWS。(b) 只对可停止型故障（ElastiCache `interrupt-az-power`）有用；本期明确推迟 `StopExperiment`，那 `Recover()` 钩子也无事可做，运维人员应急时去 AWS 控制台停就行。砍掉 CR 等于砍掉一整条 reconcile 代码路径、一份 CRD manifest、所有 webhook 和每 CR 的 RBAC——复杂度显著降低。

**考虑过的备选方案**：
- *保留 CR，用 `mode: create|start|delete` 字段拆分*：否决——CR 生命周期与"删除模板"对不齐（删 CR 要么删模板，违背持久化目标；要么不删，CR 就失去意义）。mode 字段也违背 reconciler"观察期望状态"的范式。
- *新建 `AWSFISTemplate` CRD*：否决——K8s 原生资源映射 AWS 托管状态会引入漂移 bug（AWS 控制台删了模板，CR 还在）。dashboard 直连让 AWS FIS 成为唯一事实来源。

### D2. `actionTable` 迁移：controller → dashboard 服务

**决策**：把 `controllers/chaosimpl/awsfischaos/actions.go`（`actionTable`、`targetSelection`、`actionMeta`、`goDurationToISO8601`）逐字搬到 `pkg/dashboard/apiserver/aws/fis/actions.go`。同一张 map，同字段，同辅助函数。dashboard 的 `POST /aws/fis/templates` handler 查这张表来构造 `CreateExperimentTemplateInput`。

**理由**：这张表是纯数据——actionId → resourceType / targetKey / selection / actionParameters / targetParameters。是 controller 包里本期仍需要保留的唯一一块。迁移而非重写，可以完整保留 controller 测试已经覆盖过的 action 映射契约。

**考虑过的备选方案**：
- *在 dashboard 重新实现一遍*：否决——容易跟 AWS FIS action 契约静默漂移；现有表已经评审过、测过。
- *抽到共享包 `pkg/awsfis/`*：否决——本期后只有一个调用方（dashboard 服务），共享包是过早抽象。

### D3. 五个端点，不做批量，不做 Stop

**决策**：只实现五个端点——`GET /aws/fis/templates`、`POST /aws/fis/templates`、`DELETE /aws/fis/templates/:id`、`POST /aws/fis/templates/:id/start`、`GET /aws/fis/experiments`。本期不做 `POST /aws/fis/experiments/:id/stop`。

**理由**：每个端点一一对应一次 AWS FIS API 调用，理解成本低。`Stop` 被推迟是因为用户明确决定本期不加（ElastiCache 可停止型故障是唯一受影响场景，限制已记入文档）。

**考虑过的备选方案**：
- *六个端点含 `Stop`*：用户决策否决；留给后续 change。
- *把 `templates/:id/start` 和 `experiments` 合并成一个工作流*：否决——"启动 + 轮询"是两个操作，dashboard 自己有导航状态。

### D4. 幂等 token

**决策**：
- `POST /aws/fis/templates` 用 dashboard 生成的 UUID 作为 `ClientToken`。每次 HTTP 请求生成一个新 UUID；UI 重试会带新 UUID，从而创建一个新模板（模板对运维可见，重复优于静默去重）。
- `POST /aws/fis/templates/:id/start` 用 dashboard 生成的 UUID 作为 `ClientToken`，**不再用** chaos-resource UID（没有 CR）。同样的逻辑：重试会创建新实验，可接受，因为每个新实验都有独立的 `experimentId`，运维人员看得到，也停得掉（今天走 AWS 控制台，等 `Stop` 上线后走 dashboard）。

**理由**：没有 CR UID 后没有自然的幂等键。每次请求强制用新 UUID 是最简单的契约；AWS FIS 的 `ClientToken` 在 TTL 内仍会去重，所以手抖的快速双击有保护，而刻意重试是可见的。

**考虑过的备选方案**：
- *用"模板 ID + 时间戳"哈希派生 `ClientToken`*：否决——不透明、难调试。UUID 是 AWS SDK 的惯用法。

### D5. 凭证与 client 构造复用

**决策**：新 FIS 服务复用 dashboard 现有的凭证加载路径：从 `*config.ChaosDashboardConfig` 读 `AWSFISConfig`，用 `awscfg.WithRegion(fisCfg.Region)` + `awscfg.WithCredentialsProvider(...)`（当 `SecretName != ""` 时从 K8s Secret 取）组装 AWS SDK config。`newFISClient` helper 是 controller 的 `newFISClient`（`impl.go:270-295`）和 dashboard 现有 `newRDSClient`（`aws.go:144-174`）的小幅改编。

**理由**：两个 client（RDS 用于列举、FIS 用于模板/实验）共享同一凭证来源。复用现有模式让认证面保持不变。

### D6. 给启动的实验打 tag

**决策**：`POST /aws/fis/templates/:id/start` 给启动的实验打两个 tag：`Name = "chaos-mesh/<template-id>-" + ClientToken UUID 前 8 字符`、`chaos-mesh/template-id = <template-id>`。这样 `/aws/fis/experiments` 列举（以及未来的 `Stop` 工作）能把实验关联回模板。

**理由**：没有 CR UID 可以打 tag（旧流程打 `chaos-mesh/uid = <chaos-resource UID>`），现在关联走模板。同时打 `Name`（人可读）和 `template-id`（机器可关联）覆盖两种需求。

**考虑过的备选方案**：
- *不打 tag*：否决——运维无法定位哪个实验来自哪个模板，未来 `Stop` 流程得列举全部。
- *只打 `Name`*：否决——靠字符串解析反推模板 ID 太脆。

### D7. 前端路由

**决策**：
- `/fis` → 新的 `FISList` 页（模板表 + 顶部 `[New Experiment Template]` `[Experiment Info]` 按钮 + 每行 `[Start] [Delete]`）。
- `/fis/new` → 当前 `pages/FIS/index.tsx` 的内容，略改：提交改为调 `POST /aws/fis/templates`（不再 `POST /api/experiments` 创建 CR）。成功后跳回 `/fis` 并 toast。
- `/fis/experiments` → 新的 `FISExperiments` 页，调 `GET /aws/fis/experiments`。
- sidebar "AWS FIS" 入口指向 `/fis`（新的列表页），不再指向 `/fis/new`。

**理由**：用户明确要求。列表页是天然落地页，因为可复用模板就在那里。

### D8. AWS IAM 权限说明（不涉及 K8s RBAC）

**决策**：dashboard 调用 AWS FIS 用的是 AWS 凭证（K8s Secret 里的 access key / secret key / session token），不是 K8s RBAC。所以 helm chart 里的 K8s `Role` / `ClusterRole` **不需要**追加任何 `fis:*` 权限——那些是 AWS IAM 权限，配置在 AWS 账号里的 IAM Role 或 IAM User 上，不在 chart 里。AWS IAM 权限要求写入 `helm/chaos-mesh/README.md` 让运维去 AWS 配置。

**需要的 AWS IAM actions**（在 dashboard 用的凭证对应的 IAM principal 上授权）：
- `fis:ListExperimentTemplates`
- `fis:CreateExperimentTemplate`
- `fis:DeleteExperimentTemplate`
- `fis:StartExperiment`
- `fis:ListExperiments`
- `rds:DescribeDBClusters` / `rds:DescribeDBInstances`（用于 target 下拉，沿用旧 RDS 列举端点）

本期**不**需要 `fis:StopExperiment`（无 Stop 按钮）。

**理由**：澄清"helm chart dashboard Role 追加 fis:* 权限"的初版设想是错的——AWS FIS API 调用走 AWS SDK 的 HTTP 签名，K8s RBAC 不管这条路径。把权限要求放在 README 是最自然的做法。

**考虑过的备选方案**：
- *在 chart 里加 IRSA annotation 自动配置*：否决——IRSA 配置跟集群的 IAM OIDC provider 强耦合，chart 不应替运维做这个决定。README 指引 + 运维自己配 IAM Role 才是正解。

## Risks / Trade-offs

- **[风险] 集群中已存在的 `AWSFISChaos` CR 在 CRD 删除后变孤儿** → 缓解：见下文 Migration Plan；helm chart 删除 CRD 不会自动删现有 CR 实例（K8s 行为），运维人员升级前需执行 `kubectl delete awsfi​schaos --all --all-namespaces`。chart 也可以加一个 pre-delete hook，发现仍有 CR 时告警。

- **[风险] ElastiCache 可停止型实验无法从 dashboard 停止** → 缓解：记为已知限制；运维人员可从 AWS FIS 控制台停。后续 change 会补 `Stop` 按钮 + `fis:StopExperiment` 权限。

- **[风险] dashboard 授权用户可以删除 AWS 账号下任意 FIS 模板** → 缓解：沿用 dashboard 现有认证层；按动作授权明确是 Non-Goal。如需，未来可按 tag 限定范围。

- **[风险] `actionTable` 迁移时静默偏离 controller 行为** → 缓解：逐字迁移（不重写）；现有 `actions_test.go` 也搬到 dashboard 包，对同一张表跑测试。

- **[权衡] 模板会在 AWS FIS 中持续累积** → 这正是持久化的本意，但意味着运维现在要负责模板的清理。列表页的 `Delete` 按钮是主要清理工具。

- **[权衡] FIS 实验不再有 controller events** → 运维失去 Chaos Mesh event 流对 FIS 实验的可观测。可接受，因为 dashboard 的实验页是天然的可观测入口。

## Migration Plan

1. **升级前检查**：运维执行 `kubectl get awsfi​schaos -A` 看是否还有 CR。如果有：
   - 从 AWS FIS 控制台停止所有进行中的实验（CRD 删除后 `Recover()` 不会再跑）。
   - 删除 CR：`kubectl delete awsfi​schaos --all --all-namespaces`。
2. **升级**：部署新版 helm chart。CRD manifest `chaos-mesh.org_awsfischaos` 被移除；K8s 在没有 CR 实例的前提下允许 CRD 被删除。
3. **升级后**：验证 dashboard `/fis` 页能加载，`GET /aws/fis/templates` 返回账号下已有的模板（旧 CR 流程创建的模板在 Apply 时已被删除，所以预期列表为空，除非运维在 AWS 控制台外创建过模板）。
4. **RBAC**：dashboard pod 的 IAM role（与 helm `Role` 是分开的）必须包含新增的五项 `fis:*` 权限。写入 chart README。

**回滚**：回退到上一个 chart 版本即可。CRD manifest 恢复；被删的 CR 无法找回，但运维可以从 dashboard（或 AWS 控制台）重建模板并启动实验。因为本 change 是删除而非改造 CRD，回滚很干净：CRD 直接回来。

## Open Questions

- 无阻塞项。用户已回答全部四个设计问题（Q1–Q4），并确认本期不加 `Stop` 按钮。
- 后续：另起一个 change 引入 `fis:StopExperiment` 和 `/aws/fis/experiments` 上的 `Stop` 按钮，覆盖可停止型故障。
