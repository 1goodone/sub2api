# HTTP/2 residuals vs official Codex CLI hyper

The Windows inspector capture of `codex-cli` 0.146.0 completed rustls ClientHello
but **not** HTTP/2 SETTINGS: rustls offered ECH and Go `crypto/tls` failed the
handshake (`error decrypting message` / `BadSignature`). `P2.json` therefore has
empty `http2.settings` and `connection.reuse=per_request`.

Go `golang.org/x/net/http2.Transport` can apply:

| Setting | Applied |
|---|---|
| MAX_FRAME_SIZE (5) | `MaxReadFrameSize` |
| MAX_HEADER_LIST_SIZE (6) | `MaxHeaderListSize` |

Not applied without forking `x/net/http2`:

- HEADER_TABLE_SIZE
- ENABLE_PUSH
- MAX_CONCURRENT_STREAMS
- INITIAL_WINDOW_SIZE / connection WINDOW_UPDATE
- header table / HPACK dynamic table size
- PRIORITY frames
- pseudo-header order

Until a later inspector session records `reuse=pool`, the plugin keeps
**per-request `NewClientConn`** and `ValidateConfig` rejects `connection_reuse=pool`.
Two `account_id`s with the same SNI and proxy never share a TCP/TLS connection.
