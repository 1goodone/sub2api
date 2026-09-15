# Release captures

This directory is the **embed path** for official Codex CLI rustls+hyper fixtures.

Commit here only:

- `capture.manifest.json` with `method: local_inspector`
- `P2.json` with `raw_client_hello_hex` from the inspector (reuse probe included)
- `portraits.json` (`slots` P2–P6, `filled` as measured, `fallback` P2)

Do **not** put `fake_*` files, pcap dumps, or `SSLKEYLOGFILE` sessions here.
`go test -tags=capture_release ./portrait/...` requires a real `P2.json` from official `codex-cli` 0.146.0.

The current Windows inspector capture has TLS 1.3 rustls ClientHello (including `X25519MLKEM768` key_share) but empty HTTP/2 SETTINGS because ECH prevented a completed handshake with Go `crypto/tls`. `connection.reuse` is `per_request`.

Work-in-progress inspector output: `work/` (gitignored).
