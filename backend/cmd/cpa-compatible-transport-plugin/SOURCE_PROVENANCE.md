# Source provenance

- Sub2API plugin protocol: this repository, compatible range `>=0.1.179 <0.2.0`.
- CPA behavior reference: CLIProxyAPI `v7.2.142`.
- CPA tag commit: `1f53b2eb03b9e963bac647e5566ca2b304239116`.
- uTLS profile: `tls.HelloChrome_Auto` from `utls v1.8.2`.
- Application protocol: a dedicated HTTP/2 connection per request.

The transport does not generate credentials, solve challenges, alter OAuth
identity, change request headers, or bypass application-level authorization.
