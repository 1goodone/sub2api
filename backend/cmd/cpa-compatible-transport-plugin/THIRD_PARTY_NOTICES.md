# Third-party notices

This plugin implements the Chrome uTLS and HTTP/2 behavior documented in
CLIProxyAPI `v7.2.142`, tag commit
`1f53b2eb03b9e963bac647e5566ca2b304239116`, specifically:

- `internal/runtime/executor/helps/utls_client.go`
- `sdk/proxyutil/proxy.go`

CLIProxyAPI is MIT licensed. The exact MIT notice is included as
`licenses/CLIProxyAPI-MIT.txt`.

The plugin links against Sub2API's public plugin protocol and runtime, which
are distributed under LGPL-3.0. The Sub2API license and the licenses/notices
for every Go module present in the compiled dependency graph are collected by
the release tool and included under `licenses/` in the signed package.

The implementation uses `github.com/refraction-networking/utls v1.8.2` and
`golang.org/x/net/http2 v0.56.0`. All third-party components retain their own
copyright and license terms. This plugin is not an official CLIProxyAPI or
Sub2API release.
