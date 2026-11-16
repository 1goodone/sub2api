# Ranxi v2.21.3 through v2.21.5 integration

## Scope and acceptance

- Date: 2026-11-16 (Asia/Shanghai). Baseline: 9c4fd505ee59b856002005fa523a08f590d71054.
- Import the cumulative release delta after v2.21.2 (dbed7697aa56e76c6eb48c4914fa441496b67bdd).
- v2.21.3: aa8e596e6d6f3e4648d6beba73b39b240dd5357a.
- v2.21.4: 0e8e97aa23afd8e88a61ea64a84a8fb84f1bfdc6.
- v2.21.5: b9feadf7dd6228a4117e360821d9994b7cc54897. The remote release tags were checked again before completion.
- Work on codex/merge-ranxi-v2213-v2215-20261116; preserve the separate original checkout and its existing edits. Exclude unreleased upstream production commits.
- Acceptance: compile all unit-tagged backend packages and the embedded Linux application; run affected frontend, backend, real PostgreSQL/Redis and browser checks; preserve local cache/financial/history policies; reproduce baseline failures separately; leave no conflict markers.
- Publication follows the existing authorization to push main. This integration does not deploy production instances or change worker origins.

## Resolution decisions

- Preserve per-model cache-read rules, requested-model matching, fixed/dynamic reduction, explicit zero overrides, original upstream billing counters and the OAuth quota cost estimate. Repair native Anthropic streaming at complete SSE event boundaries, including multiline usage; incomplete terminal frames cannot hide HTTP/2 resets.
- Preserve local scoped priority scheduling, reservation, admission and financial controls. Do not import unrelated scheduler/RPM implementations that conflict with the existing runtime.
- Preserve portable full encrypted history by default, with immutable input/control proofs, strict response pointers and pending/delivery fences. Explicitly enabling the 409 readable-recovery setting selects upstream owner checks and readable rebuilding; this opt-in must not silently disable the imported feature. Shadows and epochs never authorize replay of uncertain work or overwrite a known original producer when another account consumes intact history. First-seen intact input may establish a compatibility observation only after successful delivery, matching the fork baseline. A dedicated unit test checks both policies.
- Restore ticket-bound harvest egress and session identity for WS pool and passthrough acquisition; retain and release managed egress leases around the upstream session.
- Add platform usage snapshots alongside local model-provider filters. Preserve named timezones and use signed SQL intervals for unnamed fixed offsets. Verify both half-open boundaries and bucket labels.
- Preserve Cloudcat denominations, cancellation/recovery policy, bonus exclusion and provider checks. Read the losing cancellation race through the active transaction to avoid waiting on its own database connection. Keep coupon quote amounts through payment-provider selection and check the actual discounted amount against provider fee/limits. Local BPS image admission rejections must not penalize account health.
- Import QQ Bot, mature-provider management, coupons and upstream gateway/UI fixes. SystemOne support remains excluded consistently with the existing local integration policy. Accept upstream removal of the separate balance-forecast feature; it is independent of the retained OAuth quota estimate.
- Regenerate Ent and Wire. Keep dependency manifests and lockfiles unchanged. Adapt upstream tests to local constructor and admission contracts.

## Database compatibility

- Preserve all 429 existing SQL migration files. Renumber the 37 imported migrations by +7 to avoid local migrations through 349.
- 350-351: usage platform; 352-355: mature providers; 357-382: QQ Bot; 383: coupons; 384-385: QQ rewards; 386: customer cost details; 387: sidebar default. Number 356 is unused.
- Retain migration 314 and the existing model-cache data schema/checksum. Update imported migration references together with the renumbering.
- Upgrade a disposable database initialized by the previous release test image through the full candidate chain. No production migration is executed by this task.

## Verification

- Affected frontend suites: 1,478 passing, two failing, no skips. Both failures are the unchanged CreateAccountModal whitelist/mapping cases, reproduced on the baseline.
- Full frontend suite: 6,072 passing, 90 failing, 53 skipped. Every failing test and the three suite-load errors also reproduce in an archived baseline frontend snapshot using the same dependencies. No new failure is identified by that comparison; this is not an all-green frontend suite.
- Frontend typecheck and production bundle pass. Existing large-chunk/Browserslist warnings remain. The final desktop/mobile browser harness has eight screenshots, zero browser errors and passing interaction/layout assertions. The mobile group-name field was repaired after visual inspection.
- Ent and Wire generation pass. All unit-tagged Linux backend packages compile (94 packages with tests and 66 without), and the server builds with its embedded frontend. All 367 changed production Go files and all 37 new SQL migrations were SHA-256 compared with the successful Linux verification image, with no missing files or mismatches. Critical native SSE, WS-ticket and cleanup test sources were also compared.
- Additional management/repository/package checks: 174 passing test events, zero failures/skips. Service cleanup and selected-timezone unit checks pass.
- QQ Bot full Linux unit/integration run: 1,424 passing test events, zero failures, one opt-in semantic-evaluation skip; the complete QQ migration chain passes.
- Cache/quota/payment/native-terminal/WS-ticket subset: 141 passing test events, zero failures/skips, including the Cloudcat cancellation status-race regression.
- Clean isolated PostgreSQL repository regression: 197 passing test events, zero failures/skips, final PASS, including quota concurrency and signed fixed-offset timezone buckets.
- Final service regression: 2,418 passing test events, zero failures, one opt-in live moderation skip, final PASS.
- WebSocket cache rules and billed-usage regression: eight passing test events, zero failures/skips, final PASS. Checks cover per-turn rule refresh and fixed/dynamic reduction in pooled and passthrough sockets.
- The five repaired HTTP/WS policy suites: 180 passing test events, zero failures/skips, final PASS. The local scheduler rejects incompatible BPS transports with exact HTTP 503 before forwarding; the test retains exact error-body, zero-send and unchanged-owner assertions. Beta-off WS continuation preserves the exact full input; Beta-on paths retain omission and refusal boundaries.
- Final complete handler regression: 1,259 passing test events, zero failures/skips, final PASS. Covers HTTP/WS recovery, moderation scope, BPS, HTTP/2, QQ and payment boundaries. Intermediate failing/aborted logs are retained outside the repository and are not passing evidence.
- No unmerged index entries or whitespace/conflict-marker errors remain. The existing 429 SQL migrations and dependency manifests/lockfiles are unchanged. Disposable verification PostgreSQL, Redis and their dedicated network were removed after successful checks; production containers and worker origins were not modified.

## Limits

- Browser evidence uses real Vue components, locales, Tailwind and BaseDialog with synthetic DTOs and an offline HTTP adapter. It does not establish full-page or real-provider interoperability.
- No paid checkout, production traffic replay, load test or production deployment is part of this integration. Database rollback is not implied by reverting source.
- Existing frontend test debt and bundle-size warnings must remain visible in the final evidence; the integration is not a claim that all pre-existing CI is green.

## Quality self-assessment

Scores reflect this integration and the observed evidence, not production certification. Overall: 9.4/10, deliverable.

| Dimension | Score | Evidence / remaining improvement |
| --- | --- | --- |
| Functionality | 9.5/10 | Cache, quota, payment, HTTP/WS, moderation and repository gates pass; original financial policies retained. |
| Code | 9.5/10 | All 160 unit-tagged packages compile, embedded server builds, Ent/Wire regenerate, migration/lock integrity checked. |
| UI | 9.4/10 | Eight desktop/mobile screenshots and layout assertions pass; full-page integration and existing frontend test debt remain. |
| UX | 9.4/10 | Real component interactions pass in both locales; real-provider/full-page journeys remain outside this offline harness. |
| Art | N/A | No new visual assets or art direction are introduced. |
| Performance | 9.3/10 | Bounded streaming and reservation behavior covered; no load test, existing bundle-size warnings remain. |
| Security | 9.5/10 | Ownership, uncertain-delivery replay, admission, moderation scope, privacy and payment boundaries checked on isolated fixtures. |

Below-9.5 improvements: reproduce complete authenticated UI/provider journeys, address the separately reproduced frontend baseline failures, measure concurrent production-like load and reduce oversized frontend chunks. These are not claims of verification by this task.

## Evidence locations

Detailed local evidence is retained in the parent directory of this integration worktree (outside the published source):

- `v2215-linux-build11.log`: full Linux build; verification image digest `sha256:bab3d751990cc0793fd669e77053beb83538d0da57bcd7a45aa8f5d3fc149401`.
- `v2215-source-hashes.log`: verification-image source hashes.
- `v2215-linux-service-complete.log`, `v2215-linux-handler-complete.log`, `v2215-linux-handler-focused.log`, `v2215-linux-ws-cache-final.log`: final service, gateway and local-policy gates.
- `v2215-linux-cache-payment-final.log`, `v2215-linux-repository-clean.log`, `v2215-linux-qqbot.log`: payment/cache and real-database gates.
- `v2215-frontend-final-changed.json`, `v2215-frontend-final-affected.json`, `v2215-frontend-baseline-failures.json`: frontend results and exact failure-name comparison.
- `v2215-typecheck.log`, `v2215-frontend-build-final.log`, `v2215-ui-final/manifest.json`: typecheck, bundle and browser evidence.
