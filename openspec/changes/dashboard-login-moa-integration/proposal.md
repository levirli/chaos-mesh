## 背景

Chaos Dashboard 目前缺乏统一的身份认证能力，无法满足内部安全与访问控制要求。通过接入 MOA（MoonTon OAuth Authentication）登录体系，可以让 Dashboard 复用公司已有的身份与权限基础设施，确保与其他内部工具保持一致的认证体验。

## 变更内容

- 在 Chaos Dashboard 开启 MOA 模式后，未认证用户自动重定向到 MOA 登录页，不展示本地 Token 登录入口；已登录用户在顶部导航栏展示用户信息与登出入口。
- 根据配置拼装 MOA 登录 URL：`https://login.moa.moonton.net/login?project={project}&redirect={redirect}`，其中 `project` 为配置的项目 ID，`redirect` 使用 `encodeURIComponent` 编码当前 Dashboard 地址。
- MOA 登录成功后，从回调 URL 中解析 `uid`、`nick`、`name`、`token` 四个参数。
- 将 `token` 持久化到浏览器 `localStorage`，以支持页面刷新后保持登录状态。
- 每次向后端 API 发起请求时通过 HTTP Header 携带 `token`。
- 后端从请求中提取并校验 MOA token，校验方式与 MOA 框架权限验证章节保持一致。
- 在 Dashboard 配置中新增相关配置项：项目 ID、MOA 登录地址、token Header 名等。
- 更新后端 API 中间件/网关，对受保护接口进行 token 校验。

## 能力清单

### 新增能力

- `moa-login`：Dashboard 首页的 MOA 登录入口、登录 URL 拼装、回调参数解析。
- `moa-token-persistence`：浏览器端 token 的持久化存储与读取。
- `moa-token-backend-validation`：后端 API 对 MOA token 的提取与校验。

### 变更能力

- 无

## 影响范围

- Dashboard 前端（`ui/`）：登录页、路由、HTTP 客户端拦截器、浏览器存储/cookie 处理。
- Dashboard 后端（`pkg/dashboard/`）：认证中间件、配置解析、token 校验集成。
- Helm 部署配置：新增 MOA 项目 ID、登录地址等配置项。
- 不涉及 Chaos 实验 CRD、控制器或 Chaos Daemon 的改动。