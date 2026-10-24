# BPS model rate-limit recovery

This policy is enabled by **System settings → Gateway service → Excel BPS error recovery** (`excel_bps_recovery_enabled`). It applies to BPS Responses forwarding; it does not change native Codex/OpenAI retry policy or silently choose another model or protocol. Turning the switch off makes new requests ignore this policy's model backoffs. In-flight requests retain their existing switch snapshot.

## Model backoff

- Ordinary BPS HTTP 429 and in-stream rate-limit errors cool down the selected **account + mapped upstream model**. Other BPS models on the same account remain eligible.
- With no valid upstream timing hint, consecutive failures wait **30, 60, 120, 240, then 300 seconds**. A configured general 429 cooldown longer than 30 seconds remains a floor, with the backoff cap at least that floor.
- Valid structured `resets_at`, `resets_in_seconds`, or `Retry-After` take precedence over the fallback. Upstream-directed waits are bounded at two hours; an existing longer deadline is never shortened by a concurrent response.
- A fully completed BPS response resets that model's streak. An older in-flight success cannot erase a newer failure. Streaks expire after 15 idle minutes beyond the last cooldown deadline.
- Explicit account-wide HTTP quota exhaustion retains the existing quota handling. A generic `rate_limit_exceeded` body with a reset time alone does not prove account-wide exhaustion.
- These temporary model backoffs are process-local. Restarting a process clears them; separate replicas maintain independent backoffs. They do not pause accounts or change stored model permissions.

## Bounded recovery

A request may try up to **three distinct BPS accounts** when they consecutively return 429 for its locked model. A different recoverable failure resets this consecutive-429 budget. Exhaustion returns the existing client error instead of continuing through the entire account pool. This reduces tail latency during broad model throttling, with the tradeoff that a fourth account is not attempted after three consecutive 429s.

Main-request failures before client output and image-history compaction failures before generation can switch accounts. Once output has been delivered it is not replayed. Tool correction retains its existing one bounded retry of the rejected correction request; an accepted business generation is not rerun because correction was rate limited. Correction failures still update the BPS model cooldown without rewriting primary Codex quota.

## Diagnostics

Search backend system logs for:

- `openai.excel_bps_model_rate_limited`: account, mapped model, scope, original HTTP status, safe error code, cooldown source/deadline/seconds, and consecutive failure count.
- `openai.excel_bps_rate_limit_retry_exhausted`: locked model, distinct limited-account count, and account limit.

Request logger context is preserved where available. Original upstream messages, request content, and credentials are not included. Structured reset metadata travels through an internal observer before stream normalization and is not exposed in client error payloads. For in-stream failures, HTTP status may be 200 while the semantic failure is 429.

The three-account budget is independent of the usage-policy 403 failure counter. Neither this counter nor model backoff can restore upstream capacity or guarantee a successful response when all eligible accounts are limited.
