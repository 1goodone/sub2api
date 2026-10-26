# BPS model rate-limit recovery

This policy is enabled by **System settings → Gateway service → Excel BPS error recovery** (`excel_bps_recovery_enabled`). It applies to BPS Responses forwarding; it does not change native Codex/OpenAI retry policy or silently choose another model or protocol. Turning the switch off makes new requests ignore this policy's model restrictions and stops them creating new model restrictions. Turning it back on resumes any unexpired restrictions still held by the process. In-flight requests retain their existing switch snapshot.

Unavailable-model access errors use [background probe recovery](bps-model-unavailable-recovery.md), sharing the existing model rate-limit duration setting and the same recovery switch. There is no additional duration field. Rate-limit expiry behavior is unchanged.

## Configurable model restriction

- Ordinary BPS HTTP 429 and in-stream rate-limit errors cool down the selected **account + mapped upstream model**. Other BPS models on the same account remain eligible.
- In the same Gateway service card, **Model rate-limit cooldown (minutes)** (`excel_bps_model_rate_limit_cooldown_minutes`) accepts integer values from **1 to 43200 minutes**. The default is **10 minutes** when no valid value is saved. Upgrades preserve previously saved values, including 1440 minutes; change the existing setting to use a different shared interval. The saved duration is persisted in system settings and does not require a restart.
- A model rate limit restricts that account/model for the configured duration from the failure, not until calendar midnight. Additional in-flight failures use the then-current setting, but cannot shorten an existing later deadline. The model becomes eligible automatically when the deadline expires.
- Changing the duration applies to newly observed rate limits; existing cooldown deadlines are preserved. API updates that omit the setting preserve its stored value, and out-of-range/non-integer inputs are rejected. Unreadable or malformed stored settings use the 10-minute default.
- Upstream reset times, `Retry-After`, and the general 429 cooldown setting do not override this BPS-specific configured duration. Safe numeric body reset hints remain available in diagnostics only; no duration arithmetic is performed on untrusted hint values.
- A successful response cannot clear an active restriction, even if it completes after the limit was observed. Only a successful business attempt started after expiry clears the old streak. Observation-only requests cannot clear business state. Streaks expire after 15 idle minutes beyond the last cooldown deadline.
- Explicit account-wide HTTP quota exhaustion retains the existing quota handling. A generic `rate_limit_exceeded` body with a reset time alone does not prove account-wide exhaustion.
- These temporary model restrictions are process-local. Restarting a process clears them; separate replicas maintain independent restrictions. They do not pause accounts or change stored model permissions.

## Bounded recovery

A request may try up to **three distinct BPS accounts** when they consecutively return 429 for its locked model. A different recoverable failure resets this consecutive-429 budget. Exhaustion returns the existing client error instead of continuing through the entire account pool. This reduces tail latency during broad model throttling, with the tradeoff that a fourth account is not attempted after three consecutive 429s.

Main-request failures before client output and image-history compaction failures before generation can switch accounts. Once output has been delivered it is not replayed. Tool correction retains its existing one bounded retry of the rejected correction request; an accepted business generation is not rerun because correction was rate limited. Correction failures still update the BPS model cooldown without rewriting primary Codex quota.

## Diagnostics

Search backend system logs for:

- `openai.excel_bps_model_rate_limited`: account, mapped model, scope, original HTTP status, safe error code, cooldown source (`model_limit_cooldown`)/deadline/seconds, consecutive failure count, and safe numeric body reset hints when supplied.
- `openai.excel_bps_rate_limit_retry_exhausted`: locked model, distinct limited-account count, and account limit.

Request logger context is preserved where available. Original upstream messages, request content, and credentials are not included. Structured reset metadata travels through an internal observer before stream normalization and is not exposed in client error payloads. For in-stream failures, HTTP status may be 200 while the semantic failure is 429.

The three-account budget is independent of the usage-policy 403 failure counter. Neither this counter nor the model restriction can restore upstream capacity or guarantee a successful response when all eligible accounts are limited.
