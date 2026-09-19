package portrait

// RequiredHostCodexCLIVersion 钉的是 Codex Desktop 捆绑的 rustls 引擎版本
// （WindowsApps OpenAI.Codex 里的 app/resources/codex.exe --version），
// 不是宿主网关默认 TUI 身份版本。抓包与发布 fixture 必须等于这个值。
const RequiredHostCodexCLIVersion = "0.153.4"

// RequiredDesktopAppVersion 是本次画像对应的 Codex Desktop / ChatGPT 包版本。
const RequiredDesktopAppVersion = "26.903.9818.0"

// CodexUpstreamMinVersion 与宿主 codexUpstreamMinVersion 对齐。
const CodexUpstreamMinVersion = "0.144.0"

// SchemaVersion 是 capture.manifest.json / P2.json 的 schema_version。
const SchemaVersion = 1
