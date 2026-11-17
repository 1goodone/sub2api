# 409 历史恢复后的账号额度耗尽

## 根因与修复边界

开启 409 加密历史省略后，原有 `omittedHistory` 分支把上游 HTTP 429
直接交给 `handleErrorResponse`。共享额度冷却仍会执行，但 handler 收不到
可切号的类型化错误，因此用户仍收到 429。

额度耗尽现在进入现有的统一分类、冷却与 failover 流程。只有原生 HTTP
第一次发送明确被额度拒绝，且尚无语义输出、工具输出或待发布归属时，
才签发内部的恢复重试许可。handler 使用该许可和原有切号预算。

每个候选账号继续从冻结的可读历史生成兼容请求，并在真实发送前重新校验
原历史归属、工具证明及请求摘要。不得清除 pending/failed/conflict 记录，
不得把新账号的投影覆盖为下一次重试的原始历史。

HTTP 200 流内错误、断流、取消、重复 HTTP 尝试、WS/fallback、400 恢复、
已输出内容或待发布状态不能获得这个许可。既有通用禁止重放逻辑保持有效。

## 客户端行为

| 情况 | 结果 |
| --- | --- |
| 可安全切号且后续账号成功 | 返回正常结果 |
| 额度耗尽且账号池/预算耗尽 | HTTP 503 `server_error` |
| 已提交心跳后耗尽 | 保留已提交的 HTTP 200，仅发送一个 SSE 终止错误 |
| 切号前历史归属变为 pending/failed/conflict | 发送前拒绝，返回既有 409 |
| 普通瞬时限流或用户自身限流 | 保留原有行为，不统一改为 503 |

运维记录继续保留真实上游 429。显式配置的错误透传规则仍按既有优先级执行。

## 验证

`TestOpenAI409QuotaRecovery` 覆盖标准/Lite、结构化额度错误、响应头额度信号、
切号成功、额度耗尽、预算耗尽、真实心跳、普通 429、400、流内失败与断流。

`TestOpenAI409QuotaRecoveryRedis` 使用两个独立 gateway/cache 客户端与临时
真实 Redis，覆盖跨实例恢复以及切号期间归属变更；拒绝账号不得发布归属。

`TestOpenAI409QuotaReplayPermission` 覆盖内部许可的输出、投递、传输和取消边界。

2026-11-17 验证：Linux Go 1.27.4、隔离真实 Redis，扩大回归 1,204 个测试项、
定向 `-race` 152 个测试项全部通过，均无失败或跳过（计数包含子测试）。
生产改动及测试文件的 SHA-256 已与测试容器源码核对。独立复核发现的预备快照
误判和工具测试断言问题均已修正并回归。

```sh
cd backend
# Redis 用例需要可执行的 redis-server；CI 缺少该程序会失败。
go test -race -tags 'unit httpprojection' ./internal/service ./internal/handler \
  -run 'TestOpenAI409Quota|TestOpenAIHeaderWait|TestOpenAIQuotaFirstEvent|TestOpenAIQuotaExhaustion|TestUpstreamQuotaExhaustion|TestOpenAI429FastPath' \
  -count=1 -timeout=300s
```

本修复不修改数据库结构、限流配置或部署拓扑。代码验证不等于线上生效；
发布后仍需核对最终客户端状态码、切号次数和真实上游错误，而不只查看内部 429 日志。
