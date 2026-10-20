# even.sub2api.codex-desktop-transport

OpenAI OAuth outbound transport plugin that mimics **Codex Desktop** bundled engine TLS (not the Chromium `ChatGPT.exe` shell, and not the host TUI identity).

On Windows, Desktop-bundled `codex.exe` (`codex-cli` 0.163.0-alpha.10.2) sends a **Schannel TLS 1.2** ClientHello (no GREASE, no ALPN → HTTP/1.1). Runtime (`service.go` / `transport.go`) replays that hello with uTLS `HelloCustom` from `portrait/captures/P2.json` over a **per-request** connection. Headers pass through unchanged.

Install remains disabled until plugin config `"enabled": true`. Host `Enable` refuses `enabled=false` and refuses an empty `plugins.openai_oauth_account_allowlist`. This plugin is mutually exclusive with the CLI rustls plugin and the CPA Chrome plugin.

See `tools/capture/README.md` for inspector usage. `connection_reuse=pool` is rejected until a capture with `reuse=pool` exists. Packaging: `go run ./tools/release check-embed` from this directory. linux-amd64 `.s2plugin` uses a dedicated publisher `key_id` (do not reuse the CLI rustls or CPA Chrome keys).
