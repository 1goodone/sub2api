# v2.21.6 与 Issue 892 集成记录

## 来源与范围

- 隔离分支：`codex/integrate-v2216-issue892-20261117`。
- 原部署基线：`3802abf3d86a2dd6ef9d5edca78ef4a695d7064c`；先以 `2265c813bc3d38eb96034e92116594d7fb4c58fe` 保存已部署的账号耗尽 503、TokenGuard 和迁移 388 等未提交改动。
- 发布增量：上游 v2.21.5 `b9feadf7dd6228a4117e360821d9994b7cc54897` → v2.21.6 `321f11c1a4d23302961c223ac030fd09e3f9e6e8`。
- Issue 892 修复：PR 895 `0fd8d600532bd759b2c33526db4773c615e1bc58` 相对其 production 基线 `dd84ac1da71391a9a112aec89f549c188a4ebe97` 的增量。审查时该 PR 仍是 Draft，不能视为已经合并或完成动态验收。
- 历史发布采用补丁集成，双方 Git 祖先长期分叉；本次应用发布差异并逐项处理冲突，没有合入整条上游历史。原 Documents 工作区保持原样。

## Issue 892 与安全边界

原始完整历史冻结后，账号手动时区改写导致发送输入摘要变化，从而本地返回 `encrypted_agent_recovery_wire_changed/input_integrity`。现在从不可变原始输入派生所选账号的有限时区投影，冻结该投影后继续校验真实发送内容及账号。

- 普通 user message 的环境时区可以按管理员设置改写；任务文字、密文、工具参数/结果、数组顺序及大整数不能随之变化。
- 额外修复 PR 草稿漏洞：`agent_message` 等特殊类型、顶层带密文及含密文内容段的消息不参与时区改写。受保护历史的 item 摘要不一致必须拒绝，不能在未知 owner 时提前成功并绕过发送前归属检查。
- 新增 HTTP/WS 透明传递与并发 pending-owner 检查。400 同账号恢复测试使用实际第二次请求 context，并验证原 context 不能授权删除后的历史。
- 原有响应锚点、工具结果重试约束、失败/未决归属检查与本地完整历史可携带策略保留。无新增开关或依赖。
- 旧 Issue842 测试只将合法时区投影改为接受并断言完整实际输入；namespace/system promotion 仍必须在网络发送前拒绝。
- Issue892 CI 显式从上游拉取并核对固定 RED 基线 SHA，兼容本仓库补丁集成而非完整上游祖先的历史；候选 GREEN 使用当前精确提交。

## 本地适配

- 新 SQL 迁移顺延为 `389_group_free_openai_ultrafast.sql`、`390_openai_totp_binding.sql`，修正测试/文档/工作流引用；既有 SQL 迁移不修改。Ent 从合并后的本地 schema 重新生成，保留全部本地字段与索引顺序。
- 保留缓存减免、分组/个人倍率、账号偏好、非流式限制别名、BPS 冷却与原请求体处理；在本地鉴权缓存转换器中补齐 UltraFast 字段。
- UltraFast 免费开关默认关闭，只改变客户收费。保留本地默认 2 倍与 GPT-6 Astra 专属 6 倍策略；新 GPT-6.2 型号沿用本地默认倍率，不因上游测试固定 6 倍而改变既有计费规则。
- 本地 Session 导出拆分在独立文件中，已在真实 claim 路径适配 TOTP secret/factor ID 和维护代理选择；请求代理账号禁止绕过已选出口的插件路径。
- 本地独立 worker Dockerfile 也应用与托管 runtime 相同的固定源码校验和 TOTP 补丁。toSub2 必须使用 `8548397e89bf80e508eda64a87e0d556d43abc84`，旧构建 context 的源码哈希不同，会明确停止构建。
- 数据库回归发现已有取消重登 SQL 缺少 `oauth_profile`，补齐 RETURNING 字段，保留取消/迟到回调测试。旧软删除 Key 计费测试更新为当前保留行继续记账的既有规则，没有改变计费实现。
- 请求代理回归发现透传的凭据刷新继承了脱离取消的响应读取 context。令牌获取改用原请求 context，客户端取消即终止 SIWC 刷新；响应读取/用量收尾仍沿用现有语义。取消测试的 HTTP 超时设为长于断言窗口，避免把超时误认为取消传播。
- 上游测试的本地接口适配包括配额构造器、BPS context、代理查找 stub、SIWC 独立任务状态及共用 handler fixture。旧版本 RED overlay 只排除不存在依赖的无关 Issue892 fixture，当前版本测试保留。
- 本地不存在的 `openai_proxy_runtime_fallback.go` 改动已落到实际 `openai_plugin_transport.go`；未复活本地已删除的历史工作流和 Grok 测试文件。发布新增的历史工作流归档保留。

## 验证

验证在 ssh40 的隔离构建目录 `/opt/sub2api-builds/v2.21.6-issue892-20261117` 进行。测试 PostgreSQL/Redis 使用独立容器、独立数据库和回环端口，无生产卷。HTTP/WS/worker 用合成请求及本地上游，不执行真实账号登录或推理。

- 前端：类型检查通过；所选 17 个测试文件共 424 项通过；Linux `pnpm run build` 成功。部分已有 jsdom 网络日志不影响断言；没有声明整个前端测试集通过。
- 浏览器：真实 GroupsView、Toggle、NewFeatureBadge，中英文/暗色、默认关闭、保存/重载/提示关闭通过，10 张截图，pageerror 为 0。预览 fixture 补齐本地 console CSS 与 Tailwind 工作目录。
- worker：实际独立镜像构建成功，110 项 Python 测试及 28 项 Node 测试通过，涵盖固定协议模块、TOTP 绑定、加密 journal、真实 HTTP 客户端的合成服务。Windows journal 权限测试不适用，最终以 Linux 结果为准。
- RED：未修改的上游 v2.21.6 后端加当前两份 Issue892 测试文件，12 个 HTTP + 18 个 WS 场景均因完整性策略拒绝而失败，无编译失败。两文件与候选源码的哈希相同。GREEN：对应 Issue892 场景全部通过。
- 格式与差异：变更 Go 文件 gofmt 检查、`git diff --check`、冲突标记检查通过。
- 数据库：全新隔离数据库执行完整迁移，所选 256 项 repository/计费/TOTP/TokenGuard/分组/代理测试全部通过，无跳过；另 3 项迁移并发、幂等及 schema 一致性检查通过。
- CI 辅助脚本：11 项任务选择测试、Linux 下 8 项历史 fixture 测试、4 项代理配置切换证据测试及 3 项 Issue881 RED 证据测试通过。
- 可复核性：暂存候选代码在提交前测试；本地与 Linux 验证目录的 backend/frontend/tools 共 8,105 个受 Git 跟踪文件逐一核对 SHA256，差异为 0。浏览器 manifest 中的 HEAD 是当时基线，实际页面来自暂存候选源码，不是原基线页面。

- 后端：最后候选运行 `unit,bps_request_integration`，service/handler/admin/dto/repository/mihomo 所选测试共 2,254 项通过、0 失败；真实 Redis 子项因未设置地址跳过。随后指定独立 Redis 的回环端口 26379，父项及 miniredis/真实 Redis 两个子项共 3 项通过、无跳过，补齐该项。包含 SIWC 刷新取消、租约释放、无额外推理，以及原响应 context 行为。
- 构建：最后候选 `go build -tags embed ./cmd/server` 成功；Linux 前端生产包已嵌入。验证镜像 `sub2api:v2.21.6-issue892-final-verify` 是构建环境镜像，不当作生产运行镜像发布。
- 竞态：Issue892/842/805/请求时区首轮 `-race` 共 464 项全部通过、无跳过。最后取消修复后的 Issue892 HTTP/WS、SIWC 配置切换/取消与 Copilot 取消语义复查共 100 项通过、无失败/跳过/竞态告警。
- 原始证据：工作树上一级的 `issue892-verified-evidence.tar.gz` 保存最终后端/数据库/竞态/RED/构建 JSON、退出码、worker/前端/CI 日志与源码 SHA256 清单；`screenshots/` 保存浏览器截图。RED 的 32 个失败事件为 30 个预期失败场景及其 2 个父项，不是候选失败。

## 后续审查修复

后续审查确认的四项配额、请求代理和 TOTP 并发问题及修复验证，见 [审查修复记录](review-fixes-20261117.md)。扩大回归发现的基线失败仍未闭环；下文历史质量评分不能解释为当前候选已通过全仓发布门禁。

## 交付边界

本次是独立分支集成，不包含发布上线；线上仍是此前部署版本。未执行真实供应商登录、真实推理或生产压力测试。默认关闭的新恢复/免费开关不自动启用。

迁移 390 的新 TOTP 绑定数据写入后，不能只回退旧 server/worker：旧代码不识别版本化密文和绑定确认协议。发布应成套升级服务端与 worker，并保留数据库、journal 及加密密钥；免费 UltraFast 应在旧进程退出后再启用。详见 `openai-totp-factor-recovery.md`、`free-ultrafast-billing.md`。

## 质量自评

以下分数评估本次集成范围，不代表全产品审计或生产可用性保证。

| 维度 | 分数 / 10 | 证据或限制 |
| --- | --- | --- |
| 功能 | 9.5 | Issue892 修复前后对照、计费与迁移回归、取消请求和代理出口检查 |
| 代码 | 9.5 | 固定上游提交、局部适配、格式/构建、源码哈希一致性 |
| UI | 9.4 | 真实分组页面中英文/暗色截图；未做全站及移动端视觉回归 |
| UX | 9.4 | 默认关闭、保存/重载/提示交互通过；未做真实账号新恢复流程的端到端操作 |
| 美术 | N/A | 沿用现有组件与样式，本次无新视觉设计 |
| 性能 | 9.3 | 并发/租约与取消回归；未做生产规模压力测试与延迟基准 |
| 安全 | 9.5 | 不透明历史与 owner 边界负向测试、TOTP 绑定与 journal 检查、隔离测试资源 |
| 整体 | 9.4 | 集成可交付；上线前仍需按部署说明完成环境验证 |

未达 9.5 的改进点：补充移动端/全站视觉回归、真实授权流程验收以及生产规模压测。这些检查未执行，不作为已完成证据。
