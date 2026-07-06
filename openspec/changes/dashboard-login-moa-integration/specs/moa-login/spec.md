## ADDED Requirements

### Requirement: 未认证用户自动重定向到 MOA 登录页
系统 SHALL 当 `moa_security_mode` 开启且用户未持有有效 MOA token 时，自动将浏览器重定向到 MOA 登录 URL，而不展示 Dashboard 首页或本地 Token 登录对话框。

#### Scenario: 未认证用户访问 Dashboard
- **WHEN** 未认证用户打开 Chaos Dashboard，且本地存储中无有效 MOA token
- **THEN** Dashboard 不展示首页内容，自动将浏览器重定向到 MOA 登录 URL

### Requirement: 登录 URL 包含 project 与 redirect 参数
系统 SHALL 按 `https://login.moa.moonton.net/login?project={project}&redirect={redirect}` 格式拼装 MOA 登录 URL，其中 `project` 为配置的项目 ID，`redirect` 使用 `encodeURIComponent` 编码当前 Dashboard 地址。

#### Scenario: 触发自动重定向
- **WHEN** 系统决定重定向未认证用户到 MOA 登录页
- **THEN** 浏览器跳转到 MOA 登录 URL，且 project 与 redirect 参数均正确编码

### Requirement: Dashboard 提取 MOA 回调参数
系统 SHALL 在 MOA 将用户重定向回 Dashboard 后，从回调 URI 中解析 `uid`、`nick`、`name`、`token` 四个查询参数。

#### Scenario: MOA 携带凭证回调
- **WHEN** MOA 将浏览器重定向回 Dashboard 的 redirect URI，并携带 `uid`、`nick`、`name`、`token` 查询参数
- **THEN** Dashboard 提取上述四个参数并保存到应用状态中

### Requirement: Dashboard 处理缺失或非法回调参数
系统 SHALL 当任一必需查询参数缺失或 `token` 为空时，不持久化任何凭证，使用户保持未认证状态。

#### Scenario: 回调缺少 token
- **WHEN** MOA 回调 URL 缺少 `token` 查询参数
- **THEN** Dashboard 不保存 token，用户保持未认证，并被重新引导至 MOA 登录页

### Requirement: Dashboard 展示已登录用户信息与登出入口
系统 SHALL 在 MOA 模式下用户已认证时，在顶部导航栏展示用户身份信息（昵称/姓名/UID）并提供登出入口。

#### Scenario: 已登录用户查看用户信息
- **WHEN** 已通过 MOA 登录的用户浏览 Dashboard
- **THEN** 顶部导航栏展示用户标识（昵称优先，其次姓名、UID）

#### Scenario: 用户登出
- **WHEN** 已登录用户触发登出
- **THEN** Dashboard 从配置存储中移除 MOA token 与用户信息，并重定向回 MOA 登录页