# 多主机调度快照职责

`runtime.scheduler_snapshot_mode` 支持 `auto`（默认）、`writer`、`reader`，也可通过 `RUNTIME_SCHEDULER_SNAPSHOT_MODE` 设置。默认 `full` 节点保持原有行为；默认 `gateway` 节点只消费共享 Redis 或已配置的 regional 调度快照，不再执行启动重建、outbox 轮询、全量重建和 regional 调度快照发布。`gateway + writer` 配置会被拒绝。

多副本部署应明确指定一个 `full + writer` 作为生产者，其余节点使用 reader；此配置不提供 writer 选主，也不会阻止两个 full writer 同时启动。现有缓存桶 epoch 和租约继续提供写入 fencing。若要切换生产者，先停用原生产者任务，再启用新生产者；本修改不包含线上切换。

reader 的候选列表、单个账号和批量完整账号读取允许缓存命中。缓存缺失、损坏或不可用时返回不可用，不捕获发布 token、不重拉账号库、不回写完整列表，即使旧配置仍启用了 `gateway.scheduling.db_fallback_enabled`。regional reader 的不可用、过期及缺失继续沿用其既有拒绝服务行为。空命中的合法候选列表仍可返回空列表。

reader 并非整个应用的数据库只读副本：鉴权、账本、并发、请求引起的账号冷却/限流反馈仍按既有路径执行。`UpdateAccountInCache` 保留即时账号状态反馈，避免禁用/限流反馈延迟。共享 Redis reader 的分组详情仍显式查询数据库；只有配置 regional 详情 reader 时才从区域快照获取分组。本修改不会把所有跨机查询消除，也不改变请求最终出口。

生产者必须先完成首轮快照，再让 reader 接流；reader 不会自动用数据库补齐。当前 `/readyz` 的 DB/Redis 连通检查不等于快照已存在或足够新鲜，不能单独作为 reader 接流门禁。接流前还需验证目标分组/平台候选桶和账号完整缓存均命中；撤掉候选桶或完整账号缓存时请求应失败且不产生账号数据库重拉。此修改没有提供跨共享 Redis 桶的版本/新鲜度 readiness，因此不声称完成可自动切流的多主机部署。

回滚源码或显式恢复 full writer 可恢复原有行为；恢复 writer 前应确认旧生产者已停用。不要为了消除不可用错误把 gateway 改成 writer 或放开数据库兜底。测试由 CI 执行；本地仅做格式和差异检查。
