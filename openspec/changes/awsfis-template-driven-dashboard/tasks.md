## 1. 后端 — 把 actionTable 迁移到 dashboard 服务

- [x] 1.1 创建包 `pkg/dashboard/apiserver/aws/fis/`，先放一个空的 `Service` 骨架（注入 logger + config + kubeCli，参考 `pkg/dashboard/apiserver/aws/aws.go` 中的 `Service`）
- [x] 1.2 把 `controllers/chaosimpl/awsfischaos/actions.go` 逐字迁移到 `pkg/dashboard/apiserver/aws/fis/actions.go`（移动 `actionTable`、`targetSelection`、`actionMeta`、`goDurationToISO8601`）；把对 `*v1alpha1.AWSFISChaosSpec` 的引用改为本地 spec 请求结构体
- [x] 1.3 把 `controllers/chaosimpl/awsfischaos/actions_test.go` 迁移到 `pkg/dashboard/apiserver/aws/fis/actions_test.go`；适配测试 fixture 到新请求结构体；确保 `go test ./pkg/dashboard/apiserver/aws/fis/...` 通过

## 2. 后端 — FIS client 构造

- [x] 2.1 在 `pkg/dashboard/apiserver/aws/fis/` 下新增 `newFISClient(ctx, fisCfg) (*fis.Client, error)`（改编自 `controllers/chaosimpl/awsfischaos/impl.go:270-295` 和 `pkg/dashboard/apiserver/aws/aws.go:144-174`）；沿用 `WithRegion` + `WithCredentialsProvider` 模式
- [x] 2.2 把 `github.com/aws/aws-sdk-go-v2/service/fis` 从 e2e-test / controller 的 go.mod 提升为 dashboard（根模块）的直接依赖（若尚未是）；运行 `go mod tidy`

## 3. 后端 — 五个 FIS 端点

- [x] 3.1 在 `pkg/dashboard/apiserver/aws/fis/types.go` 中定义请求/响应模型：`CreateTemplateRequest`（action、description、targetArn、forceFailover、resourceTags、availabilityZoneIdentifier、duration）、`TemplateSummary`（id、description、creationTime、tags——`ListExperimentTemplates` 不返回 actions/targets map，故不含 action/targets 字段）、`ExperimentSummary`（id、templateId、state、creationTime、tags）
- [x] 3.2 实现 `GET /aws/fis/templates` → 分页 `ListExperimentTemplates`，逐项投影为 `TemplateSummary`（AWS 的 summary 不含 actions map，故不投影 action）
- [x] 3.3 实现 `POST /aws/fis/templates` → 按 `actionTable` 校验（byArn action 必须有 targetArn，byTags action 必须有 resourceTags，含 duration 参数的 action 必须有 duration），构造 `CreateExperimentTemplateInput`（ClientToken = UUID、RoleArn 取自 config、StopConditions = `[{none}]`、AccountTargeting = single-account、EmptyTargetResolutionMode = fail、tags = `{Name: "chaos-mesh/<random>"}`），提交，返回 `{id}`
- [x] 3.4 实现 `DELETE /aws/fis/templates/:id` → `DeleteExperimentTemplate`；`ResourceNotFoundException` 映射为 404
- [x] 3.5 实现 `POST /aws/fis/templates/:id/start` → 生成 UUID 作为 ClientToken，构造 tags `{Name: "chaos-mesh/<id>-<前缀>", "chaos-mesh/template-id": "<id>"}`，调用 `StartExperiment`，返回 `{id}`
- [x] 3.6 实现 `GET /aws/fis/experiments` → 分页 `ListExperiments`，投影为 `ExperimentSummary[]`
- [x] 3.7 在 `pkg/dashboard/apiserver/aws/aws.go` 的 `Register`（或新 `fis.Register`）里挂载全部五个端点到 `/aws` group；补 swagger 注解

## 4. 后端 — 删除 AWSFISChaos CRD 和 controller

- [x] 4.1 删除 `api/v1alpha1/awsfischaos_types.go`、`api/v1alpha1/awsfischaos_webhook.go`、`api/v1alpha1/awsfischaos_webhook_test.go`、`api/v1alpha1/awsfischaos_groupversion_info.go` 以及任何 `zz_generated.deepcopy.*awsfischaos*` 文件
- [x] 4.2 删除整个 `controllers/chaosimpl/awsfischaos/` 目录
- [x] 4.3 从 `controllers/` 下移除 awsfischaos reconciler 注册（找到引用 `awsfischaos.NewImpl` 的 `fx.Provide` / controller 注册项并删除）
- [x] 4.4 从 `controllers/config/` 的 `enabled-controllers` 默认值或 RBAC 列表中移除 awsfischaos 条目
- [x] 4.5 删除 `helm/chaos-mesh/crds/chaos-mesh.org_awsfischaos.yaml`
- [x] 4.6 移除 `helm/chaos-mesh/templates/` 下与 awsfischaos 相关的 RBAC（controller Role / RoleBinding 中授权 `awsfischaos` 资源的条目）
- [x] 4.7 运行 `make generate && make manifests/crd.yaml`，确认没有 awsfischaos 产物被重新生成；修复任何残留引用

## 5. 后端 — helm chart dashboard 权限（AWS IAM，非 K8s RBAC）

- [x] 5.1 确认 dashboard 的 K8s `Role` / `ClusterRole` **无需改动**——`fis:*` 是 AWS IAM 权限，dashboard 调用 AWS FIS 走 AWS SDK 的 HTTP 签名，不经 kube-apiserver，K8s RBAC 不管这条路径（见 design D8）
- [x] 5.2 因此**不要**在 chart 里加任何 `fis.*` 相关的 K8s RBAC 规则（AWS FIS 不是 K8s apiGroup）
- [x] 5.3 在 `helm/chaos-mesh/README.md` 中写明 dashboard 用的 AWS 凭证（IAM principal）所需的五项 `fis:*` action，以及**不**需要 `fis:StopExperiment`

## 6. 后端 — 测试与检查

- [x] 6.1 用 mock FIS client 为五个 handler 加单测（覆盖 spec 中的成功路径 + ResourceNotFound / 校验失败路径）
- [x] 6.2 运行 `make check`，修复 fallout（fmt、vet、lint、gosec、deepcopy 重新生成）
- [x] 6.3 运行 `make test`，确保没有测试引用被删的 CRD 或 controller 包
- [x] 6.4 运行 `make swagger_spec`，重新生成 `pkg/dashboard/swaggerdocs/`，覆盖五个新端点

## 7. 前端 — API client 与类型

- [x] 7.1 在 `ui/app/src/api/`（或现有 `/aws/rds/clusters` 调用所在位置）添加类型化封装：`listFIStemplates()`、`createFIStemplate(body)`、`deleteFIStemplate(id)`、`startFISexperiment(templateId)`、`listFISexperiments()`
- [x] 7.2 若项目从 swagger 生成前端类型，重新生成；否则手写 TypeScript 类型，对齐新的后端响应结构

## 8. 前端 — `/fis` 模板列表页

- [x] 8.1 创建 `ui/app/src/pages/FIS/List.tsx`（或把 `index.tsx` 重构成 List 组件）—— Paper / Table 布局，顶部工具栏放 `New Experiment Template`（跳转 `/fis/new`）和 `Experiment Info`（跳转 `/fis/experiments`）两个按钮
- [x] 8.2 实现模板表：列为 `ID | Description | CreationTime | Actions`（不含 Action/Targets 列——`ListExperimentTemplates` 不返回这些）；`Actions` 列每行放 `Start` 和 `Delete` 按钮
- [x] 8.3 点击 `Start` → 调 `startFISexperiment(id)`，成功 toast 后跳转 `/fis/experiments`；失败内联 alert
- [x] 8.4 点击 `Delete` → 弹确认对话框 → 确认后调 `deleteFIStemplate(id)` → 收到 204 移除该行；收到 404 显示错误并重新拉取
- [x] 8.5 空态行与错误态行（覆盖 spec 中的场景）

## 9. 前端 — `/fis/new` 页（重构现有页面）

- [x] 9.1 把现有 `ui/app/src/pages/FIS/index.tsx` 内容移到新组件（如 `ui/app/src/pages/FIS/New.tsx`）；保留 action-card + 表单布局
- [x] 9.2 修改 `handleSubmit`：改为调 `createFIStemplate(...)`，不再调 `usePostExperiments({data: payload})`（不再创建 AWSFISChaos CR）
- [x] 9.3 成功后跳转 `/fis` 并 toast；失败内联 alert
- [x] 9.4 移除 `usePostExperiments` import 和 CR 形态的 `payload` 构造

## 10. 前端 — `/fis/experiments` 页

- [x] 10.1 创建 `ui/app/src/pages/FIS/Experiments.tsx` —— 表格列为 `Experiment ID | Template ID | State | CreationTime`
- [x] 10.2 挂载时调 `listFISexperiments()`；数组为空时渲染空态
- [x] 10.3 本期不放 `Stop` 按钮（按 design 中的 Non-Goal）

## 11. 前端 — 路由、菜单、i18n

- [x] 11.1 更新 `ui/app/src/router.tsx`：把单一 FIS 路由替换为三条（`/fis` → List、`/fis/new` → New、`/fis/experiments` → Experiments）
- [x] 11.2 更新 sidebar 菜单项，让 "AWS FIS" 指向 `/fis`（不再指向 `/fis/new`）
- [x] 11.3 更新 `ui/app/src/i18n/zh.json` 与 `en.json`——为新按钮（`New Experiment Template`、`Experiment Info`、`Start`、`Delete`）和新页面标题加文案；保留现有 "AWS FIS" 菜单文案
- [x] 11.4 若 sidebar 用到 route map 或 breadcrumb config，同步更新，让进入 `/fis/new` 时面包屑显示 "AWS FIS / 新建模板"（或等价文案）

## 12. 前端 — 测试与构建

- [x] 12.1 为 List 页加 vitest 单测（mock API：渲染行、点击 Start、点击 Delete 并确认）
- [x] 12.2 为 New 页提交加 vitest 单测，确认调用的是 `createFIStemplate`（而不是 CR 端点）
- [x] 12.3 运行 `cd ui/app && pnpm build`，修复 TS / lint 错误
- [x] 12.4 在浏览器中对着 dev 集群（或 mock API）冒烟测试三个页面

## 13. 文档与迁移说明

- [x] 13.1 更新 `helm/chaos-mesh/README.md`（或相关文档），写明 dashboard 新增 `fis:*` IAM 要求，以及 design 中 Migration Plan 的步骤
- [x] 13.2 在 change 的 design.md（或 change 目录下新增 `MIGRATION.md`）中加一条说明，指示运维升级前执行 `kubectl delete awsfi​schaos --all --all-namespaces`
- [x] 13.3 确认 change 的 `proposal.md` 的 Why 段落中包含 `Supersedes: add-awsfischaos-fault-injection` 引用（已存在）

## 14. 最终验证

- [x] 14.1 `make check` 通过
- [x] 14.2 `make test` 通过（无测试引用被删的 CRD / controller）
- [x] 14.3 `cd ui/app && pnpm build` 通过
- [x] 14.4 端到端手测：创建模板 → 在 `/fis` 看到它 → 启动实验 → 在 `/fis/experiments` 看到 → 删除模板 → 该行消失
- [x] 14.5 确认 chaos-controller-manager 不再注册 awsfischaos reconciler；确认 dashboard 的 K8s `Role` / `ClusterRole` **未新增**任何 `fis:*` 条目（`fis:*` 是 AWS IAM 权限，非 K8s RBAC，见 design D8）

## 15. 详情页 + 更新 + 列表互跳(增量)

- [x] 15.1 后端 types:`TemplateDetail`、`ExperimentDetail`(`types.go`)
- [x] 15.2 后端 handler:`GET /aws/fis/templates/:id`(GetExperimentTemplate)、`PUT /aws/fis/templates/:id`(UpdateExperimentTemplate)、`GET /aws/fis/experiments/:id`(GetExperiment);抽 `resolveTemplateInputs` 供 create/update 共用;`ResourceNotFoundException`→404;未知 action→400
- [x] 15.3 后端单测:路由注册补 3 条 + updateTemplate 校验(未知 action / 缺 targetArn → 400)
- [x] 15.4 README 补 `fis:GetExperimentTemplate`、`fis:UpdateExperimentTemplate`、`fis:GetExperiment` 三项 IAM 权限
- [x] 15.5 前端 API:`getFIStemplate`、`updateFIStemplate`、`getFISexperiment` + `FISTemplateDetail`/`FISExperimentDetail` 类型
- [x] 15.6 前端详情页:`TemplateDetail.tsx`(只读展示 + 编辑 description/target,更新/返回)、`ExperimentDetail.tsx`(只读 + 关闭)
- [x] 15.7 前端列表:`List.tsx`「Experiment Info」→「Experiment」+ ID 可点;`Experiments.tsx` 加「Experiment Templates」按钮 + ID 可点;`router.tsx` 加两条详情路由
- [x] 15.8 端到端验证(真实 AWS,msdk-fis / ap-southeast-1):`GET /aws/fis/templates/:id` 200 返回 action/target;`PUT /aws/fis/templates/:id` 200(改 description 已验证并还原);`GET /aws/fis/experiments/:id` 200 返回 state/action;模板详情页(编辑表单预填)、实验详情页(只读+关闭)、两列表互跳按钮 + 可点 ID 均已渲染

## 16. 停止实验(增量)

- [x] 16.1 后端:`POST /aws/fis/experiments/:id/stop` → `StopExperiment`;`stopExperimentResponse` 类型;`ResourceNotFoundException`→404;路由注册测试补该路由
- [x] 16.2 README IAM 补 `fis:StopExperiment`;spec.md 加「停止实验」requirement 并改掉「列举实验」里的「SHALL NOT Stop 按钮」;IAM requirement 补 `fis:StopExperiment`
- [x] 16.3 前端:`stopFISexperiment(id)`;`Experiments.tsx` 加 Actions 列,非终态(pending/initiating/running)行显示 `Stop` + 二次确认,成功刷新列表
- [~] 16.4 端到端验证(部分):Stop 端点已挂载(无 token 401、路由非 404);对一个 completed 实验调用返回 AWS `ValidationException: Cannot modify the state of an experiment in final state`(证明路由 + `fis:StopExperiment` 权限可用,非 AccessDenied);前端实验列表已有 Actions 列,终态实验不显示 Stop 按钮(状态门控正确)。**未实测**:按钮出现→点击→停止一个运行中的实验(账号当前无运行中实验,且未带 AWSFIS_ROLE_ARN 无法启动新实验)

## 17. ElastiCache 按 ARN 选目标 —— 已放弃(AWS 不支持),回退到 tag

**结论(真实 AWS 验证得出)**:AWS FIS 对 `aws:elasticache:replicationgroup` 依次拒绝了两种非 tag 方案:
1. `resourceArns` → `ValidationException: TargetResourceType aws:elasticache:replicationgroup does not support resourceArns`
2. 仅 `filters`(按 ReplicationGroupId)→ `ValidationException: Tags are required for TargetResourceType aws:elasticache:replicationgroup`

即该资源类型**强制按 tag 选目标**,无法用 ARN/filter。ARN 改造整体回退。

- [x] 17.1 回退 `actions.go`:ElastiCache 保持 `targetByTags`;移除 `targetByFilter`/`filterPath`
- [x] 17.2 回退 `handlers.go`:移除 `resolvedInputs.filters`、`targetByFilter` 分支、target 的 `Filters`、`strings` import
- [x] 17.3 回退 `aws.go`:移除 elasticache client / `GET /aws/elasticache/replication-groups` / import,`newRDSClient` 恢复原样;`go mod tidy` 移除 elasticache SDK
- [x] 17.4 回退前端:`TargetKind`/`targetEndpoint` 去掉 elasticache;interrupt-az-power 恢复 `hasResourceTags`(tag 输入)
- [x] 17.5 回退 README/spec:ElastiCache 说明改回 tag 选择(补充 AWS 强制 tag 的原因)
- [x] 17.6 端到端验证:RDS reboot 建模板 create 200 / delete 204(RoleArn 已修复);ElastiCache interrupt-az-power 用 tag(resourceTags={Name:...})建模板 create 200 / delete 204;New 页 ElastiCache 表单恢复 Resource tags 输入(无 ARN 下拉),Duration 为数字+单位组合

> 注:RoleArn(`AWSFIS_ROLE_ARN`)缺失是本轮 create 报错根因,已在运行的后端补上 `arn:aws:iam::429937351945:role/msdk-fis-role`,RDS 建模板已验证可用。

## 18. 新增 DynamoDB global-table-pause-replication(增量)

**可行性实测**:与 ElastiCache 不同,FIS 的 `aws:dynamodb:global-table` **支持按 ARN 选目标**(用真实 AWS CreateExperimentTemplate + resourceArns 验证通过);target key = `Tables`,参数仅 `duration`。仅适用于 global table(replicas>0)。

- [x] 18.1 加依赖 `aws-sdk-go-v2/service/dynamodb`;`types.go` 加 `DynamoDBPauseReplication` Action 常量
- [x] 18.2 `actions.go` 加 `DynamoDBPauseReplication`(resourceType `aws:dynamodb:global-table`、targetKey `Tables`、targetByArn、oneshot=false、duration→ISO8601);更新 `actions_test.go` 覆盖
- [x] 18.3 `aws.go` 加 `GET /aws/dynamodb/global-tables`(ListTables+DescribeTable,**仅返回 replicas>0 的 global table**);重引入共用 `loadAWSConfig` + `newDynamoDBClient`
- [x] 18.4 前端:index.tsx 加 DynamoDB group + action(ARN 下拉 + duration 数字/单位);`TargetKind`/`targetEndpoint` 加 `dynamodb-globaltable`;api `FISAction` 加新 id
- [x] 18.5 README:dashboard 补 `dynamodb:ListTables`/`dynamodb:DescribeTable`;FIS role 补 pause-replication 所需权限;spec 加 DynamoDB 创建场景
- [x] 18.6 端到端验证:`GET /aws/dynamodb/global-tables` 返回 200 + 空数组(账号 ap-southeast-1 无 global table,权限可用);New 页出现第 4 张卡片「Pause Global Table Replication」(DynamoDB 分组),点开显示「DynamoDB global table ARN」下拉 + Duration(数字+单位),无 AZ/tags。**真正建模板/跑实验需先有 global table**(当前账号无,下拉为空)
