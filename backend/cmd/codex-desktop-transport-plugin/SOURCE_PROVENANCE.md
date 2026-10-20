# Source provenance

- Plugin ID: `even.sub2api.codex-desktop-transport`
- Capability: `openai.oauth.outbound_transport.v1`
- Desktop app: Windows Store/AppX `OpenAI.Codex` 26.1013.4894.0 (`app/ChatGPT.exe` is Chromium; not captured)
- TLS: uTLS `HelloCustom` from Desktop-bundled `codex.exe` (`codex-cli 0.163.0-alpha.10.2`) Windows Schannel TLS 1.2 ClientHello (`portrait/captures/P2.json`, JA3 `6a5d235ee78c6aede6a61448b4e9ff1e`)
- HTTP: HTTP/1.1 (captured hello has no ALPN); per-request connections
- Proxy dialer: same CONNECT/SOCKS5 patterns as the CPA-compatible plugin
- Headers: host-owned; this plugin does not rewrite `User-Agent`, `originator`, `version`, or `x-codex-routing-hint`

The transport does not generate credentials, solve challenges, alter OAuth
identity, change request headers, or bypass application-level authorization.
