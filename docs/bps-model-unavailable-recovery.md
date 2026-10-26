# BPS unavailable-model recovery

The existing **System settings → Gateway service → BPS error recovery** switch controls model isolation, request failover, and background model recovery. Only BPS account/model routes are affected. Native OpenAI/Codex routes and other models/accounts remain eligible. Failover retains the requested mapped model; this feature does not downgrade the model.

## Shared cooldown setting

- The existing unavailable-model classification is retained: HTTP 403 with `basispoints_model_access_changed` / `model_not_allowed`, or HTTP 400/404 with `model_not_found` / `model_not_found_error`, including recognized in-stream failures.
- These errors isolate the selected **account + mapped upstream model**, then use the existing bounded failover path where client output has not started.
- Reuse the existing **Model rate-limit cooldown (minutes)** setting (`excel_bps_model_rate_limit_cooldown_minutes`) for both model rate limits and unavailable-model probes. The default is **10 minutes**, with an integer range of 1–43200. There is no second duration field. Previously saved values are preserved on upgrade; selecting 10 in the existing field applies the 10-minute interval to both policies.
- Saving an interval applies to new failures and subsequent failed-probe cooldowns. Already scheduled deadlines are preserved; a later failure cannot shorten a longer deadline.
- Elapsed time never releases an isolated model. After its deadline, the background worker sends a separate authenticated BPS Responses request using the exact upstream model and a generated nonce. It does not replay a user's request. A completed response containing exactly the nonce is required to restore routing.
- Rejected, incomplete, malformed, timed-out, or incorrect probe responses keep the model isolated. Another test is scheduled after the configured interval. A newer model failure invalidates any older in-flight probe result.

## Switches and lifecycle

Turning error recovery off makes new requests ignore this isolation and stops new background tests. An in-flight successful probe cannot clear its entry while the switch is off. Turning the switch on again resumes pending entries, including overdue ones. Disabling the global BPS protocol switch also stops new probes. Normal requests keep the existing per-request switch snapshot.

The worker checks for due models every 15 seconds and tests up to three per scan, with a 45-second timeout. A busy queue may defer a test beyond the configured minimum interval. It rereads the account before testing, skips paused/expired accounts, disabled BPS routes, and active account/model rate limits, and cancels/joins on service shutdown. It uses the account's configured proxy, including managed BPS proxy leases; it does not fall back to a direct route when the configured proxy pool is unavailable.

Invalid credentials continue to pause the whole account under the existing error recovery policy. Model probe success only removes its own model isolation; it cannot resume a manually paused account or clear a separate rate-limit restriction. Logs contain account IDs, mapped model IDs, and timing, never credentials, probe responses, or user prompts.

As with the preceding model-cooldown implementation, temporary isolation is process-local: restarting clears entries, and replicas maintain independent state. Settings are persisted. The map holds up to 4096 entries; when full, new failures still follow request failover but cannot add isolation until capacity becomes available. Existing isolated entries are not evicted. Deleted accounts are removed during the next due scan.

## Diagnostics

Search system logs for `excel_bps.model_recovery_scheduled`, `excel_bps.model_recovery_probe_failed`, `excel_bps.model_recovery_restored`, and `excel_bps.model_recovery_capacity_reached`. `next_probe_at` on scheduling records is the earliest test time, not an automatic release time.
