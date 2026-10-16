# Streaming first-response mode

In System Settings > Gateway Services > Gateway Forwarding, enable the
"Use first response for first-token timing" switch. The stored setting is
`stream_first_response_enabled` and defaults to `false`. No schema migration
is required. Saving updates the runtime settings cache; each stream uses the
setting captured when it starts.

When enabled, a real upstream JSON event can start downstream output without
waiting for generated text. Metadata such as `response.created`, an assistant
role chunk, or `message_start` is forwarded using the requested protocol.
`first_token_ms` records the first real upstream event, excluding heartbeat
events, SSE comments, and `[DONE]`. Existing OpenAI TTFT settings are preserved
and take effect again after this switch is disabled.

The switch applies to HTTP SSE and Responses WebSocket forwarding, including
Chat Completions, Messages, Gemini, Antigravity, and their protocol bridges.
It does not switch the account's upstream transport, generate placeholder
answer text, or change token usage. Bridges wait for an upstream event before
emitting their initial protocol metadata in this mode.

Downstream clients that measure the first response event can observe this
earlier event. Clients that explicitly ignore metadata and wait for nonempty
answer text will still measure content arrival. The gateway cannot override
that client-side rule.

Once metadata has been sent, the request cannot be silently replayed against
another account. Subsequent failures end the current stream. An empty Responses
completion after metadata is emitted as `response.failed`, not successful empty
output. Disabling the switch restores the previous buffering and timing rules.

Regression tests cover settings persistence and cache reload, enabled/disabled
HTTP flush ordering, usage-only Chat timing, protocol metadata conversion,
post-metadata failure and empty completion, and WebSocket per-turn timing.
These tests use simulated upstreams; they are not live-provider latency results.

Validated on 2026-10-15: 351 selected Go test cases (including subtests), 67
frontend settings/locale tests, Vue type checking, and ESLint on changed frontend
files passed. Desktop (1440px) and mobile (390px) browser checks confirmed default
off, save/reload on and off, keyboard control, accessible labels, and no overlap.
The unrestricted service test suite exceeded its 120-second timeout. The `unit`
build remains blocked by existing duplicate `routingTestAccount` declarations and
outdated `handleCCStreamingFromAnthropic` calls in unrelated tests.
