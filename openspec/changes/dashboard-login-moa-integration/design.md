## 背景

Chaos Dashboard 目前已支持三种认证方式：

1. **内置 Token 登录**（`security_mode`）：用户在前端粘贴 Kubernetes token。
2. **GCP OIDC**（`gcp_security_mode`）：通过 Google OAuth 跳转，token 以 cookie 形式存储。
3. **通用 OIDC**（`oidc_security_mode`）：通过可配置的 OIDC 提供商跳转。

前端通过 `/api/common/config` 接口发现当前启用的认证方式。现有的 `Auth.tsx` 登录弹窗会根据 `gcp_security_mode` / `oidc_security_mode` 是否开启来渲染额外的登录按钮。后端 OIDC 相关逻辑位于 `pkg/dashboard/apiserver/auth/oidc/`，现有 `AuthMiddleware` 通过 `clientpool.ExtractTokenAndGetAuthClient` 校验 Kubernetes token。

本次变更新增第四种认证方式：**MOA 登录**，用于接入 MoonTon 内部 OAuth 网关。与现有 OIDC 流程不同，MOA 要求前端自行拼装授权 URL，并在跳转回 Dashboard 的地址中通过查询参数直接返回用户身份（`uid`、`nick`、`name`、`token`）。后端只需在后续 API 请求中校验该 token。

## 目标 / 非目标

**目标：**
- 支持用户通过 MOA 登录 Chaos Dashboard。
- 由前端完成 MOA 登录 URL 拼装、跳转处理及 token 存储。
- 每次访问受保护 API 时向前端后端传递 MOA token。
- 在后端受保护接口使用与 MOA 框架权限验证相同的方式校验 token。
- 通过环境变量和 `/api/common/config` 接口暴露所需配置。

**非目标：**
- 不替换或移除现有的 Token、GCP、OIDC 登录方式。
- 不在 Chaos Mesh 内部实现细粒度的 MOA 权限/角色映射（仅完成 token 校验，原有 Kubernetes RBAC 流程仍保留）。
- 不支持非浏览器客户端或机器对机器的 MOA 认证。

## 关键设计决策

### 1. 前端拼装 MOA 登录 URL 并自动重定向

**决策：** 当 `moa_security_mode` 开启且用户未认证时，由前端 `TopContainer` 的 guard effect 直接拼装 `https://login.moa.moonton.net/login?project={project}&redirect={encodeURIComponent(redirect)}`，并通过 `window.location.href` 自动跳转到 MOA 登录页。MOA 模式下不展示登录按钮、不弹出内置 Token 对话框。

**理由：** MOA 的约定要求调用方自行拼装 URL，且跳转目标为 Dashboard 自身。MOA 模式下 Dashboard 不提供本地 Token 登录手段，未认证用户没有理由停留在首页，自动重定向比"展示按钮等用户点击"更符合 SSO 体验，也避免 RBAC 对话框在跳转前闪现（guard effect 在 loading 态完成重定向）。

**备选方案：**
- 在 `Auth.tsx` 中渲染 MOA 登录按钮，由用户点击触发跳转（类似 GCP/OIDC）。此方案会让未认证用户先看到 Dashboard 首页与本地 Token 对话框，与 MOA 作为唯一认证方式的设计不符，且多一次用户交互。
- 后端提供 `/api/auth/moa/redirect` 接口来拼装 URL。此方案会增加一次请求，且 MOA 的 URL 是静态可参数化的，没有必要由后端完成。

### 2. token 经 HTTP Header 传输并持久化到 localStorage

**决策：** MOA token 通过 HTTP Header（由 `moa_token_header` 配置，默认 `Moa-Token`）传输，浏览器端持久化到 `localStorage`。不提供 cookie 传输/存储模式。

**理由：** Header 传输需由 JS 主动读取 token 并附加，`localStorage` 是其自然存储，与现有 Dashboard 状态使用 `localStorage` 的做法一致，且 Header 传输更易排查问题。cookie 模式虽能借 `HttpOnly` 降低 XSS 读取 token 的风险，但会引入 CSRF 面并需额外防护；而 Dashboard 的破坏性操作仍依赖 Kubernetes RBAC，MOA 仅建立身份，cookie 收益有限。

**备选方案：**
- 同时支持 `header` / `cookie` 两种模式由 `moa_token_transport` 切换。原设计曾采用此方案，但前端 cookie 模式未实现、且无实际用例支撑双模式，遂移除 cookie 模式以简化配置与前后端语义。
- 始终使用 cookie 以便自动传输。如上所述，CSRF 风险与有限收益使其不优先于 Header。

### 3. 后端校验层封装 MOA token 校验

**决策：** 在 `pkg/dashboard/apiserver/auth/moa/` 中新增 Gin 中间件，从配置的 Header 中提取 token，交由 `TokenValidator` 接口实现校验。生产实现 `RemoteValidator` 调用 MOA user info 端点 `https://api-ms-portal.infra.catdelta.com/user/info`（请求头 `Moa-Token: <token>`），校验返回 JSON 中的 `project_id` 与配置的 `MoaProjectId` 一致，一致则返回用户 UID。

**理由：** 需求明确说明”后台获取 token 进行验证，具体方法与 MOA 框架权限验证一节一样”。`TokenValidator` 接口便于测试时以 `SetValidator` 注入 fake，独立中间件便于隔离与复用。

**备选方案：**
- 复用现有 OIDC service。MOA 返回的是跳转 URL 上的原始 token，不遵循 OIDC 的 code-exchange 流程，因此不能复用。

**已知局限：** user info 端点地址与默认 project ID（`1600`）当前硬编码在 `validator.go`，未通过配置暴露。测试环境或不同 MOA 部署需通过 `SetValidator` 注入 fake 绕过。将端点抽为配置项属于后续改进。

### 4. 公共端点放行

**决策：** `/api/common/config` 和静态资源等公共端点在 MOA 模式下无需 token 即可访问，确保 UI 能在认证前获取到 MOA 登录配置。

**理由：** 登录流程需要被引导。前端必须预先知道 MOA 是否启用、项目 ID 和登录地址，才能拼装 URL。

## 风险 / 权衡

- **风险**：token 存储在 `localStorage` 中时，若 Dashboard 存在 XSS 漏洞，token 可能被脚本读取。
  - **缓解**：继续遵循现有 CSP 与输入净化实践；token 仅用于 MOA 身份建立，破坏性操作仍依赖 Kubernetes RBAC 凭证。
- **风险**：MOA 回调参数（`uid`、`nick`、`name`、`token`）可能保留在浏览器历史或被代理日志记录。
  - **缓解**：前端提取参数后，立即使用 `history.replaceState` 替换 URL，清除地址栏中的敏感信息。
- **风险**：MOA token 校验方法可能依赖内部闭源库或 HTTP 内省接口。
  - **缓解**：将校验逻辑封装在 `TokenValidator` 接口之后，具体实现可替换或 mock，便于开发和测试。

## 迁移计划

1. 在 `ChaosDashboardConfig` 中新增 MOA 配置字段。
2. 实现后端 `auth/moa` 中间件并条件注册。
3. 通过 `/api/common/config` 暴露 MOA 相关配置。
4. 实现前端 MOA 自动重定向 guard、URL 拼装器、回调处理器和 token 存储。
5. 更新 API 客户端拦截器以附加 MOA token。
6. 在 Helm values 和环境变量模板中新增 MOA 配置。
7. 在预发环境部署，验证登录/登出、token 持久化、token 过期后的认证失败处理。
8. 回滚：关闭 `MOA_SECURITY_MODE` 即可切回内置 Token 登录。

## 已澄清决策

原设计阶段遗留的待澄清问题，已在实现阶段定案如下：

1. **MOA token 校验方式**：调用 MOA user info 端点 `https://api-ms-portal.infra.catdelta.com/user/info`，以 `Moa-Token` 请求头携带 token，校验返回的 `project_id` 与配置一致。token 为 MOA 网关下发的原始字符串，由端点判定有效性。
2. **后端校验使用的 Header**：Dashboard 后端 → MOA 端点使用 `Moa-Token`；前端 → Dashboard 后端使用配置的 `MoaTokenHeader`（默认 `Moa-Token`）。两段 Header 名相同但语义独立。
3. **与 `security_mode` 共存**：MOA 优先。当 `moa_security_mode` 为 true 时，前端 guard effect 接管认证流程，不再弹出内置 Token 对话框（`TopContainer` 中 `data.security_mode && !data.moa_security_mode` 才走 RBAC 分支）。
4. **MOA project ID**：配置项 `MoaProjectId`，未配置时 `RemoteValidator` 回退到默认值 `1600`。