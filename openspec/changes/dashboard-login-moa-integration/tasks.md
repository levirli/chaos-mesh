## 1. 配置与后端脚手架

- [x] 1.1 在 `pkg/config/dashboard.go` 中新增 MOA 配置字段：`MoaSecurityMode`、`MoaProjectId`、`MoaLoginUrl`、`MoaTokenHeader`。
- [x] 1.2 创建 `pkg/dashboard/apiserver/auth/moa/` 包，包含 `Service` 结构体、`Middleware` 方法和 `Register` 函数。
- [x] 1.3 在中间件中实现从配置的 HTTP Header 中提取 MOA token。
- [x] 1.4 在中间件中集成 MOA 框架 token 校验逻辑，对无效或缺失 token 返回 401 Unauthorized。
- [x] 1.5 当 `MoaSecurityMode` 开启时条件注册 MOA 中间件，并将 `/api/common/config`、`/api/auth/*` 及静态资源加入白名单。

## 2. 后端配置暴露

- [x] 2.1 通过 `/api/common/config` 暴露安全的 MOA 配置字段：`moa_security_mode`、`moa_project_id`、`moa_login_url`、`moa_token_header`。
- [x] 2.2 重新生成或更新 OpenAPI/TypeScript schema，使前端类型包含新增配置字段。
- [x] 2.3 为 MOA 中间件补充单元测试，覆盖有效 token、缺失 token、无效 token 场景。

## 3. 前端 MOA 登录流程

- [x] 3.1 新增 MOA 类型与存储抽象（`ui/app/src/lib/moaStorage.ts`），基于 `localStorage` 持久化。
- [x] 3.2 在 `ui/app/src/components/TopContainer/Auth.tsx`（或新建 `MoaCallbackHandler`）中实现 MOA 回调解析，从查询字符串读取 `uid`、`nick`、`name`、`token`。
- [x] 3.3 实现 MOA 登录 URL 拼装器，对 redirect URI 使用 `encodeURIComponent` 编码。
- [x] 3.4 当 `config.moa_security_mode` 为 true 时，在 `TopContainer` 的 guard effect 中实现未认证自动重定向到 MOA 登录页（不渲染登录按钮、不弹出 Auth 对话框），并新增 `MoaUser.tsx` 展示已登录用户信息与登出入口。
- [x] 3.5 回调成功后，将 token 存入配置存储，并使用 `history.replaceState` 清除 URL 中的敏感凭证。

## 4. 前端 API token 传输

- [x] 4.1 扩展 `ui/app/src/api/interceptors.ts`，在 MOA 模式启用时从存储中读取 token 并按配置 Header 附加到请求。
- [x] 4.2 更新 `ui/app/src/zustand/auth.ts`（或新增独立 MOA auth store），暴露 `setMoaToken`、`getMoaToken`、`removeMoaToken` 等操作。
- [x] 4.3 确保登出时从配置存储中移除 MOA token，并重置 API 拦截器。
- [x] 4.4 为前端存储抽象、URL 拼装器、回调解析器补充单元测试。

## 5. 部署与文档

- [x] 5.1 在 `helm/chaos-mesh/` 中为 MOA 配置新增 Helm values 和环境变量模板。
- [x] 5.2 更新 Dashboard 文档，补充 MOA 登录接入说明。
- [x] 5.3 运行 `make check`、`make test` 及 UI 构建，确认无回归。
- [x] 5.4 在完整环境中端到端验证登录、登出、token 持久化流程。
