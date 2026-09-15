# Source provenance

- Plugin ID: `even.sub2api.codex-rs-transport`
- Capability: `openai.oauth.outbound_transport.v1`
- TLS: uTLS `HelloCustom` from official `codex-cli` 0.146.0 ClientHello (`portrait/captures/P2.json`)
- HTTP/2: Go `http2.Transport` / `NewClientConn` per request (captured SETTINGS empty because ECH blocked handshake)
- Proxy dialer: same CONNECT/SOCKS5 patterns as the CPA-compatible plugin
- Headers: host-owned; this plugin does not rewrite `User-Agent`, `originator`, `version`, or `x-codex-routing-hint`

The transport does not generate credentials, solve challenges, alter OAuth
identity, change request headers, or bypass application-level authorization.
