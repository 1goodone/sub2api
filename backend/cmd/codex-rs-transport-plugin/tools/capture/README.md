# Codex rustls / hyper capture inspector

Local TLS 1.3 + HTTP/2 inspector for **publishing** `portrait/captures/P2.json`.
It records the ClientHello (plaintext) and the first HTTP/2 SETTINGS / WINDOW_UPDATE
from official `codex-cli`, plus a **reuse probe** (≥2 requests in one CLI process).

This is not a production proxy and must not be aimed at `chatgpt.com` as a load test.

## Host CLI pin

Capture `codex_cli_version` **must equal** the host pin
`portrait.RequiredHostCodexCLIVersion` (`0.146.0`, same as `codexCLIVersion` in
`backend/internal/service/openai_gateway_service.go`), and stay ≥ `0.144.0`.

## Publish capture (hosts + mkcert)

Do **not** point the CLI at `https://127.0.0.1:9443` — that writes SNI `127.0.0.1`.
Point `chatgpt.com` at the inspector and serve an mkcert leaf for that name:

```text
mkcert -install
mkcert chatgpt.com

# inspector (admin/root if you bind :443)
go run ./tools/capture -listen 127.0.0.1:443 -cert ./chatgpt.com.pem -key ./chatgpt.com-key.pem -out ../../portrait/captures/work/ -cli-version 0.146.0

# hosts (capture window only):
# 127.0.0.1 chatgpt.com

# same official codex 0.146.0 process: >=2 harmless HTTPS requests to https://chatgpt.com
# then remove the hosts line immediately

go run ./tools/capture -decode ../../portrait/captures/work/ -require-reuse-probe
```

Copy only a complete inspector session into `portrait/captures/` (`P2.json`,
`capture.manifest.json`, `portraits.json`). `method` must be `local_inspector`.

One-shot pcap / `SSLKEYLOGFILE` belong in `portrait/testdata/debug/` (gitignored).
`tools/release` rejects those files in the embed path.

If rustls refuses the mkcert leaf (ECH / pinning): still capture ≥2 requests against
the local inspector (SNI may be a work name; runtime rewrites SNI) and set
`connection.reuse=per_request`. Do not invent `pool`.

## Windows without hosts (CONNECT proxy)

If you cannot edit `C:\Windows\System32\drivers\etc\hosts`:

```text
go run ./tools/capture -listen 127.0.0.1:9443 -connect-proxy 127.0.0.1:18080 -cert ./chatgpt.com.pem -key ./chatgpt.com-key.pem -out ../../portrait/captures/work/ -cli-version 0.146.0

# trust the generated CA (not the leaf) in the *current user* store:
certutil -user -delstore Root chatgpt.com
certutil -user -addstore Root .\cmd\codex-rs-transport-plugin\portrait\captures\work\inspector-ca.pem

$env:HTTPS_PROXY='http://127.0.0.1:18080'
$env:HTTP_PROXY='http://127.0.0.1:18080'
# pin the host CLI version — PATH `codex` may be newer
npx --yes @openai/codex@0.146.0 exec --ephemeral --skip-git-repo-check "Say hi"
```

The inspector **refuses** to publish a Schannel/curl ClientHello (TLS 1.2, no `key_share`). Only a rustls-like hello (TLS 1.3 + key_share) becomes `P2.json`. ECH outer hellos may omit ALPN.

On Windows, the most reliable driver is `-spawn-codex` so the CLI inherits `HTTPS_PROXY` pointing at `-connect-proxy`:

```text
go run ./tools/capture -listen 127.0.0.1:9443 -connect-proxy 127.0.0.1:18080 -out ../../portrait/captures/work -cli-version 0.146.0 -spawn-codex path\to\codex.exe
```

If rustls offers ECH, Go `crypto/tls` may fail the handshake (`error decrypting message` / `BadSignature`). The outer ClientHello is still a valid rustls fingerprint; HTTP/2 SETTINGS will be empty and `connection.reuse` is `per_request` from the TLS-level probe.

## Fake fixtures (CI only)

```text
go run ./tools/capture -selftest -listen 127.0.0.1:0 -out ../../portrait/testdata/fake
```

Filenames contain `fake`. They must never be copied into `portrait/captures/`.
