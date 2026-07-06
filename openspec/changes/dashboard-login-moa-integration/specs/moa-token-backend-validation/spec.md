## ADDED Requirements

### Requirement: 后端接收 MOA token
系统 SHALL 将 MOA `token` 通过 HTTP Header 从 Dashboard UI 传递到 Chaos Dashboard 后端。

#### Scenario: 通过 HTTP Header 传输 token
- **WHEN** 受保护 API 请求发起
- **THEN** 每个请求在配置的 Header（默认 `Moa-Token`）中包含 token

### Requirement: 后端提取 token 用于校验
系统 SHALL 在处理受保护 API 请求前，从配置的 HTTP Header 中提取 MOA `token`。

#### Scenario: 请求包含有效 token Header
- **WHEN** 受保护 API 请求包含已配置的 token Header
- **THEN** 后端提取 token 值并进入校验步骤

### Requirement: 后端校验 MOA token
系统 SHALL 在允许访问受保护端点前，使用与 MOA 框架权限验证相同的方法校验提取到的 MOA token。

#### Scenario: token 有效
- **WHEN** 受保护 API 请求包含有效的 MOA token
- **THEN** 后端正常处理该请求

#### Scenario: token 无效或缺失
- **WHEN** 受保护 API 请求包含无效、过期或缺失的 token
- **THEN** 后端返回 HTTP 401 Unauthorized

### Requirement: 未认证端点保持可访问
系统 SHALL 允许 MOA 登录流程所需的公共端点（如配置暴露接口、静态资源）无需 token 即可访问。

#### Scenario: 未认证用户加载登录页
- **WHEN** 未认证用户请求公共端点
- **THEN** 后端返回响应，不要求提供 token