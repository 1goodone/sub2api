# HTTP/2 residuals vs Codex Desktop

The Windows inspector capture of Desktop-bundled `codex.exe` 0.163.0-alpha.10.2 is
Schannel TLS 1.2 with no ALPN, so the session is HTTP/1.1. `P2.json` has empty
`http2.settings` because there is no HTTP/2 preface to capture, and
`connection.reuse=per_request` because the same process opened ten new
ClientHellos. Do not add HTTP/2 settings or `connection_reuse=pool` to this
portrait. `ValidateConfig` still rejects `connection_reuse=pool`. The runtime uses
HTTP/1.1 when the replayed hello has no ALPN.
