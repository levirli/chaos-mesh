## ADDED Requirements

### Requirement: Dashboard 在页面刷新后保持 MOA token
系统 SHALL 将 MOA `token` 持久化到浏览器存储中，使得认证会话在页面刷新后仍然有效。

#### Scenario: 登录后用户刷新页面
- **WHEN** 已登录用户刷新 Chaos Dashboard
- **THEN** Dashboard 从存储中恢复此前保存的 `token`，无需再次执行 MOA 登录

### Requirement: token 持久化到 localStorage
系统 SHALL 将 MOA `token` 持久化到浏览器 `localStorage`。

#### Scenario: 登录后写入 token
- **WHEN** MOA 回调携带有效 token
- **THEN** token 被写入 `localStorage`

### Requirement: 登出时清除 token
系统 SHALL 在用户登出时删除已保存的 `token`。

#### Scenario: 用户登出
- **WHEN** 用户触发登出
- **THEN** Dashboard 从配置的存储中移除 token，并将用户状态置为未认证

### Requirement: 发起 API 请求前读取已保存的 token
系统 SHALL 在每次认证 API 请求前读取持久化的 token，并将其转发给后端。

#### Scenario: API 客户端准备请求
- **WHEN** Dashboard HTTP 客户端发送 API 请求
- **THEN** 请求包含从配置存储中读取的 token