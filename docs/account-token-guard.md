# 账号凭证守护（智能运维 → 凭证守护）

面向 OpenAI OAuth 账号的令牌健康巡检与自愈能力，作为「智能运维」下的子页面提供：

- 页面路径：`/admin/token-guard`
- 接口前缀：`/api/v1/admin/account-ops/token-guard/*`
- 默认关闭：代码内置参考测活/重登服务 https://session.ameng2027.xyz（含对应请求头与模型 gpt-6-astra），
  启用前确认参数即可；也可以改成任意兼容该协议的第三方服务。

## 工作方式

1. **定时巡检**：按配置间隔用账号当前的 `access_token` 调用「测活接口」，结果分为 `正常`、`令牌失效`、`临时异常`。
2. **令牌失效自愈**：命中失效且开启自动重登时，用配置的 `邮箱 + 密码 + 2FA 密钥` 调「重登接口」换新凭据，随后写回凭据（保留 `model_mapping` 等字段）→ 清错误态 → 恢复调度 → 失效服务端 token 缓存。
3. **错误态自愈**：探活正常但账号仍为 `error` 状态时，自动清除错误态并恢复调度。
4. **可观测**：账号最近一次探活结果与巡检/修复日志全部落库，页面展示并支持单账号手动重登。

## 配置项

| 字段 | 说明 |
| --- | --- |
| 启用守护 | 关闭后不做任何巡检与修复 |
| 守护分组 | 留空表示全部 OpenAI OAuth 账号；页面通过下拉菜单支持多选分组 |
| 巡检间隔 / 超时 / 并发 / 每轮上限 | 站点测活较慢时用「每轮上限」控制单轮规模，未覆盖账号下一轮优先 |
| 测活接口 / 测活模型 | 默认指向参考服务 session.ameng2027.xyz 与 gpt-6-astra，可替换 | `POST {probe_endpoint}`，请求体 `{"access_token": "...", "model": "..."}`，按 NDJSON 返回 `{"type":"result","payload":{...}}` |
| 测活 / 重登 额外请求头 | 每行一个 `Header-Name: value`；值里可用 `{{uuid}}` 生成每次请求唯一的随机串；接口协议要求由配置提供，代码不内置任何第三方标识 |
| 自动重登 / 重登接口 / 重登凭据 | 页面每行使用 `账号----密码----2FA`（兼容旧逗号格式）；接口请求体为 `{"action":"start","email":"...","auth_mode":"password_2fa","password":"...","mfa_secret":"..."}`，返回 `credential` 对象 |
| 修复后恢复调度 | 修复成功后重新打开账号调度 |
| Bark Key / 推送开关 | 可选，修复成功或失败时推送通知 |

## 数据表

- `account_token_guard_states`：每个账号最新巡检状态（探活结果、延迟、连续失效次数、最近修复）。
- `account_token_guard_events`：巡检与修复日志，保留最近 14 天。

## 安全说明

- 巡检只读取账号已有的 `access_token`，不会主动修改账号凭据。
- 仅在「令牌失效且开启自动重登」时写回新凭据，并保留账号既有配置字段。
- 重登凭据保存在管理员设置中，请只在自建环境填写；对外共享数据库前建议先清空该字段。

## 凭证守护 V2

V2 与上面的 V1 独立存在，页面位于 `/admin/token-guard-v2`。管理员从现有 OpenAI OAuth 母账号中选择需要巡检的账号，并为每个账号单独配置一种登录方式：

- `邮箱 + 密码 + 可选 TOTP 密钥`
- `邮箱 + OTP 邮箱 API URL`

V2 默认每 30 分钟巡检一次；鉴权失败后 5 分钟复检，连续 2 次鉴权失败才创建 durable 重登任务，同一账号有活跃任务时不会重复创建。巡检使用数据库租约和 `FOR UPDATE SKIP LOCKED`，可在多实例部署中运行。

密码、TOTP 密钥和邮箱 URL 使用现有 `SecretEncryptor` 加密保存，管理接口只返回“是否已配置”和脱敏 URL。部署前必须设置固定的 TOTP 加密密钥，并为 worker 配置至少 32 字符的 `OPENAI_REAUTH_WORKER_TOKEN`。

### Worker 运行要求

- 邮箱链接模式：继续使用 `CODEX_PROTOCOL_ROOT` / `TURB_ROOT` 指向 Turb 协议目录。
- 密码/TOTP 模式：额外设置 `TOSUB2_ROOT`，指向包含 `src/protocol-login.mjs` 的 [toSub2](https://github.com/poxiao33/toSub2) checkout，并使用 Node.js 20 或更高版本。
- Windows 启动脚本可传 `-ToSub2Root C:\path\to\toSub2`。私网邮箱 API 必须使用 HTTPS，并通过 `-TrustedOtpHosts mail.internal` 精确放行主机名；未配置时仍拒绝所有非公网解析结果。
- Docker 运行时将 toSub2 checkout 只读挂载到容器，并设置 `TOSUB2_ROOT` 指向挂载目录；现有 worker 镜像已包含 Node.js 20。

账密和 TOTP 只通过子进程环境变量传给 toSub2，不出现在命令行参数或管理 API 响应中。toSub2 生成的临时 OAuth 文件在任务结束后随临时目录删除；后端仍执行账号身份匹配、旧凭据 CAS 和原子写回。
