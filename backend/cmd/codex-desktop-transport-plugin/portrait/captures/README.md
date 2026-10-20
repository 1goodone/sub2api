# Release captures

This directory is the **embed path** for the Codex Desktop Schannel fixture.

Commit here only:

- `capture.manifest.json` with `method: local_inspector`
- `P2.json` with `raw_client_hello_hex` from the inspector (reuse probe included)
- `portraits.json` (`slots` P2–P6, `filled` as measured, `fallback` P2)

Do **not** put `fake_*` files, pcap dumps, or `SSLKEYLOGFILE` sessions here.
`go test -tags=capture_release ./portrait/...` requires a real `P2.json` from Desktop `codex.exe` 0.163.0-alpha.10.2.

Work-in-progress inspector output: `work/` (gitignored).
