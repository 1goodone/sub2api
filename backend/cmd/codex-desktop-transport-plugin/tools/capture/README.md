# Codex Desktop rustls / hyper capture inspector

Local TLS 1.3 + HTTP/2 inspector for **publishing** `portrait/captures/P2.json`.
It records the ClientHello (plaintext) and the first HTTP/2 SETTINGS / WINDOW_UPDATE
from Codex Desktop's bundled rustls engine (`app/resources/codex.exe`), plus a
**reuse probe** (≥2 requests in one process).

Do not capture `app/ChatGPT.exe` / `chrome.dll`. That is Chromium, not the
Codex inference stack.

This is not a production proxy and must not be aimed at `chatgpt.com` as a load test.

## Host pin

Capture `codex_cli_version` **must equal**
`portrait.RequiredHostCodexCLIVersion` (`0.153.4`, the Desktop-bundled
`codex.exe --version`) and stay ≥ `0.144.0`. Also record
`codex_desktop_app_version` (`26.903.9818.0`).

## Windows CONNECT proxy (recommended)

```text
go run ./tools/capture -listen 127.0.0.1:9443 -connect-proxy 127.0.0.1:18080 -out ../../portrait/captures/work -cli-version 0.153.4 -desktop-app-version 26.903.9818.0 -spawn-codex path\to\codex.exe
```

Trust the generated CA (not the leaf) in the *current user* store:

```text
certutil -user -addstore Root .\portrait\captures\work\inspector-ca.pem
```

The inspector **refuses** to publish a Schannel/curl ClientHello (TLS 1.2, no `key_share`). Only a rustls-like hello (TLS 1.3 + key_share) becomes `P2.json`. ECH outer hellos may omit ALPN.

If rustls offers ECH, Go `crypto/tls` may fail the handshake (`error decrypting message` / `BadSignature`). The outer ClientHello is still a valid rustls fingerprint; HTTP/2 SETTINGS will be empty and `connection.reuse` is `per_request` from the TLS-level probe.

## Fake fixtures (CI only)

```text
go run ./tools/capture -selftest -listen 127.0.0.1:0 -out ../../portrait/testdata/fake
```

Filenames contain `fake`. They must never be copied into `portrait/captures/`.
