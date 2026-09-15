# even.sub2api.codex-rs-transport

OpenAI OAuth outbound transport plugin that mimics official Codex CLI **rustls + hyper**.

Runtime (`service.go` / `transport.go`) sends uTLS `HelloCustom` from `portrait/captures/P2.json` (official Codex CLI 0.146.0 rustls ClientHello) over a **per-request** HTTP/2 or HTTP/1.1 connection. Headers pass through unchanged.

Install remains disabled until plugin config `"enabled": true`. Host `Enable` refuses `enabled=false` and refuses an empty `plugins.openai_oauth_account_allowlist`.

See `tools/capture/README.md` for inspector usage and `docs/runbooks/codex-rs-transport-rollout.md` for rollout. `connection_reuse=pool` is rejected until a capture with `reuse=pool` exists. Packaging: `go run ./tools/release check-embed` from this directory. linux-amd64 `.s2plugin` uses a dedicated publisher `key_id` (do not reuse the CPA Chrome key).
