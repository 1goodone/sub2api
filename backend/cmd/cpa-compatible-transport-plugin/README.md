# CPA-Compatible OpenAI Transport

Auditable Sub2API transport plugin for OpenAI OAuth traffic. It reproduces the
Chrome uTLS plus HTTP/2 connection behavior used by CLIProxyAPI `v7.2.142` and
passes host-generated request headers and bodies through unchanged.

## Trust model

The Ed25519 public key and its SHA-256 fingerprint are public release data.
The private key stays on the release machine and is never copied into the
plugin package or the Sub2API server. Each Sub2API deployment must explicitly
trust the public key unless a future host release embeds it.

```yaml
plugins:
  allow_unsigned: false
  trusted_publishers:
    even-sub2api-cpa-transport-v1: "BASE64_ED25519_PUBLIC_KEY"
```

## Test and release

Run from `backend/cmd/cpa-compatible-transport-plugin`:

```text
go test . -count=1
go run ./tools/release keygen -private /secure/path/publisher.private -public /secure/path/publisher.public
go run ./tools/release package -private /secure/path/publisher.private -key-id even-sub2api-cpa-transport-v1 -output /release/path/cpa-compatible-transport-v0.1.0-linux-amd64.s2plugin
```

The package command cross-compiles Linux amd64, signs the exact manifest,
collects dependency licenses, emits publisher metadata and SHA-256 checksums,
and creates a source snapshot. It refuses to overwrite existing release files.

Installation starts in Sub2API's disabled state. The plugin configuration also
defaults to `{"enabled":false}`; enable it only after checking the signature,
health status, and account rollout binding.
