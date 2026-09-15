package portrait

// RequiredHostCodexCLIVersion 必须等于宿主 openai_gateway_service.go 的
// codexCLIVersion。抓包 CLI 与发布 fixture 都钉这个值；不一致则 capture_release
// 与 tools/release 失败。当前宿主 pin 是 0.146.0。
const RequiredHostCodexCLIVersion = "0.146.0"

// CodexUpstreamMinVersion 与宿主 codexUpstreamMinVersion 对齐。
const CodexUpstreamMinVersion = "0.144.0"

// SchemaVersion 是 capture.manifest.json / P2.json 的 schema_version。
const SchemaVersion = 1
