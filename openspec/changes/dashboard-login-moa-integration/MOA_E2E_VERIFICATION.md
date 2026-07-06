# MOA 登录接入端到端验证方案

## 前置条件

1. 已部署 Chaos Mesh（Helm 安装），且 Chaos Dashboard 可访问。
2. 已获取 MOA 项目 ID（如 `1600`）。
3. MOA 登录服务可用：`https://login.moa.moonton.net/login`。
4. MOA 用户信息接口可用：`https://api-ms-portal.infra.catdelta.com/user/info`。

## 部署配置

在 Helm values 中启用 MOA 登录：

```yaml
dashboard:
  moaSecurityMode:
    enabled: true
    projectId: "1600"
    loginUrl: "https://login.moa.moonton.net/login"
    tokenHeader: "Moa-Token"
```

升级 Helm release 后，确认 Dashboard Pod 环境变量：

```bash
kubectl exec -it deploy/chaos-dashboard -- env | grep MOA
```

预期输出包含：

```
MOA_SECURITY_MODE=true
MOA_PROJECT_ID=1600
MOA_LOGIN_URL=https://login.moa.moonton.net/login
MOA_TOKEN_HEADER=Moa-Token
```

## 验证步骤

### 1. 配置接口暴露

访问 `/api/common/config`，确认返回包含 MOA 字段：

```bash
curl https://<dashboard-host>/api/common/config
```

预期包含：

```json
{
  "moa_security_mode": true,
  "moa_project_id": "1600",
  "moa_login_url": "https://login.moa.moonton.net/login",
  "moa_token_header": "Moa-Token"
}
```

### 2. 登录入口

打开 Dashboard 首页，未登录状态下应自动重定向到 MOA 登录页（不展示本地 Token 登录对话框）。

### 3. 登录 URL 拼装

浏览器应跳转至：

```
https://login.moa.moonton.net/login?project=1600&redirect=https%3A%2F%2F<dashboard-host>%2F%23%2Fdashboard
```

其中 `redirect` 已使用 `encodeURIComponent` 编码。

### 4. 回调参数处理

完成 MOA 登录后，浏览器跳转回：

```
https://<dashboard-host>/#/dashboard?uid=1001374&nick=levirli&name=xxx&token=xxx
```

Dashboard 应：
- 成功提取 `uid`、`nick`、`name`、`token`。
- 将 token 持久化到 `localStorage`。
- 使用 `history.replaceState` 清除 URL 中的敏感参数。
- 刷新页面后仍处于登录状态。

### 5. API 请求携带 Token

登录后访问任意页面（如实验列表），通过浏览器开发者工具查看 API 请求：

```
GET /api/experiments
Moa-Token: <moa-token>
```

### 6. 后端 Token 校验

使用无效 token 访问受保护接口：

```bash
curl -H "Moa-Token: invalid-token" https://<dashboard-host>/api/experiments
```

预期返回 HTTP 401 Unauthorized。

### 7. 公共端点放行

未登录时访问 `/api/common/config` 和静态资源，应返回 200。

### 8. 登出

点击登出后：
- 清除 `localStorage` 中的 MOA token。
- 重置 API 请求拦截器。
- 再次访问受保护接口返回 401。

## 常见问题

### 回调后页面空白或参数未清除

确认 Dashboard 使用 hash router，且后端 `NoRoute` 返回 `index.html`。

### 后端返回 401，但前端已登录

检查 `MOA_TOKEN_HEADER` 配置是否一致。前端发送 token 的 Header 名必须与后端 `MoaTokenHeader` 配置相同（默认 `Moa-Token`）。

### 用户信息接口调用失败

确认 Dashboard Pod 能访问 `https://api-ms-portal.infra.catdelta.com/user/info`。如存在网络隔离，需配置代理或调整网络策略。

## 本地开发验证

本地启动 Dashboard 时，可设置环境变量启用 MOA：

```bash
MOA_SECURITY_MODE=true \
MOA_PROJECT_ID=1600 \
MOA_LOGIN_URL=https://login.moa.moonton.net/login \
MOA_TOKEN_HEADER=Moa-Token \
go run ./cmd/chaos-dashboard
```

前端本地开发：

```bash
cd ui
pnpm install --frozen-lockfile
pnpm start
```

如需本地 mock MOA 用户信息接口，可临时将 `RemoteValidator.endpoint` 指向本地服务，或实现一个 `TokenValidator` 注入到 `Service` 中。
