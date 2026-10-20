export default {
  "protocol": "OpenAI 请求协议",
  "codex": "Codex（保留现有 BPS / Prism 配置）",
  "siwc": "SIWC API（Sign in with ChatGPT）",
  "protocolHint": "授权完成后可随时切换，保存账号后生效。SIWC 使用 ChatGPT 套餐调用官方 Responses API，Codex 凭据独立保留。",
  "bound": "SIWC 已绑定（凭据存在，不代表实时可用性）",
  "unbound": "SIWC 尚未绑定",
  "automatic": "使用已保存的密码 / 2FA 授权",
  "browser": "浏览器授权",
  "refresh": "刷新状态",
  "loginHint": "自动授权复用凭据守卫中已保存的密码和 TOTP。首次同意授权或遇到额外验证时，请使用浏览器授权；后续自动刷新令牌。",
  "running": "SIWC 授权任务处理中…",
  "continue": "Continue with ChatGPT",
  "callbackHint": "在新页面完成授权后，浏览器会跳转到本机回调地址。即使提示无法连接，也请复制地址栏的完整地址并粘贴到下方。",
  "callback": "完整回调地址",
  "complete": "完成绑定",
  "failed": "SIWC 操作失败，请刷新后重试"
}
