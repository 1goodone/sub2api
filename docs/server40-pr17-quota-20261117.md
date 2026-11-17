# Server40 PR17 and Quota Recovery Deployment

## Scope and Source

Authorized rollout: both server40 applications, rebind both external workers,
and push the reviewed integration to Even1997/sub2api main.

- Application source: `0867dc6cf2ae56b88a688927fa676a513892b21a`.
- Application image: `sub2api:v2.21.6-pr17-quota-20261117`.
- Image ID: `sha256:53c7d6169c59df83d2a747b35ef886adee9222618c62b56a0b921fb9f2370b17`.
- Deployment-only follow-up commits do not change the image application source.
- PR17 was automatically marked merged after its ancestry reached main.
- GitHub accepted normal fast-forward pushes through the administrator exception
  to its PR requirement. Protection settings were not changed; no force push.

## Traffic and Workers

Successful cutover: November 17, 2026, approximately 21:03 Asia/Shanghai.

| Instance | Container | Host port | Worker | Concurrency |
| --- | --- | --- | --- | --- |
| Main | sub2api-v2.21.6-pr17-quota-20261117 | 18303 | sub2api-openai-reauth-worker | 3 |
| MR | sub2api-mr-v2.21.6-pr17-quota-20261117 | 18304 | sub2api-openai-reauth-worker-mr | 2 |

Both workers use their corresponding green application's network namespace.
Their existing image is unchanged: `sub2api-openai-reauth-worker:v2.21.6-issue892-fix-20261117`.
The worker source matches the release after line-ending normalization. Worker
environment, restart policy, limits, and each recovery archive/key were preserved.
Each recovery directory contained one file; hashes matched before startup.

Application environment, data mounts, restart policy, resource limits, and group
cache pricing settings were preserved. Both database migration signatures remained
unchanged (Main 502 records, MR 470 records). Latest recorded migration remains
`390_openai_totp_binding.sql`. Worker enabled state was restored to true in both.

## Validation

- Two complete database dumps decoded with `pg_restore --file=/dev/null`;
  application data gzip archives verified. This was not a restore exercise.
- Production frontend type/build checks and embedded backend build passed.
- All 25 audited Go file hashes match the Linux-tested sources.
- Prior exact-source validation: 2,203 Linux tests, 204 race tests, 16 real
  application scenarios; no failures or skips in those scopes.
- All nine original/PR17 deployment gates passed; final PR17 gates also cover asset
  dependency traversal, Vite root paths, and exclusion of external SDK references.
- Four public health/version checks passed; anonymous Responses requests returned
  401. All four domains' entry JS/CSS matched green origin bytes.
- Both internal MR entries on port 18180 passed health and version checks.
- Read-only authenticated model list requests returned 200 on both applications.
  Counts vary with availability; simultaneous blue/green comparisons matched.
- Both applications and workers had zero restarts, zero OOM events, and zero
  structured error/fatal/panic events during the recorded verification window.
- Worker authentication, origin version, current heartbeats, environment, mounts,
  limits, and recovery archives passed checks. Active Caddy configuration contains
  only green targets for these routes.
- 404 blue frontend assets were fetched from each running blue origin, verified,
  and preserved for already-open pages.

## Deployment Corrections

Initial prepare attempts stopped before creating green applications: the old
builder image had been retired, and new-build asset fingerprints differed.
The final script archives the actual running blue asset dependency graph instead.
Prepare failures restored worker enablement automatically.

The first cutover reload was rejected because the new Caddy file inherited 0600
permissions from the restricted runner. Caddy kept its old active configuration,
and all four public health checks stayed healthy. The corrected script preserves
file ownership/mode during atomic replacement and rollback. Current Caddyfile is
root:caddy 0640 and readable by the service. Retry completed successfully.

## Backups and Rollback

Private backup: `/var/backups/sub2api/v2.21.6-pr17-quota-20261117` (0700).
It contains database dumps, app data, private container inspections, worker
recovery archives, Caddy configuration, and the corrected deployment script.

Evidence: `/opt/sub2api-builds/v2.21.6-pr17-quota-20261117`.
The image's frozen source archive remains separate from deployment-only fixes;
the root `deploy.sh` and backup script contain the executed final workflow.

Rollback first requires `sudo bash /var/backups/sub2api/v2.21.6-pr17-quota-20261117/deploy.sh rollback-compatible`,
then the same script with `rollback`. It pauses claims and refuses active login
tasks or incompatible TOTP/billing state. Preserve post-cutover recovery records;
do not restore old database dumps over new business data.

Both blue applications reached zero established inbound connections and stopped
normally with exit code 0 at approximately 21:11 Asia/Shanghai. Old applications,
workers, and images remain available for rollback. No existing stream was stopped
to accelerate cleanup. Post-retirement checks passed against both green instances.

## Quality and Limits

This rollout does not assert whole-repository CI success. Historical RED jobs
reference missing source objects, broad lint has legacy findings, and the full
repository diff lint timed out. No real provider generation/login, production
load test, full visual regression, or database restore exercise was performed.

| Dimension | /10 | Evidence or improvement |
| --- | --- | --- |
| Functionality | 9.5 | Source tests, real app scenarios, live route/worker checks |
| Code | 9.4 | Regression/race and deploy gates; legacy CI/lint not resolved |
| API experience | 9.5 | New route verification and existing connection drain |
| Performance | 9.3 | Bounded retry/concurrency tests; production load test remains |
| Security | 9.5 | Private backups, recovery integrity, fail-closed rollback gates |
| UI / art | N/A | Backend/deployment change; static assets checked, no design changes |
| Overall | 9.4 | Deliverable within stated scope; not whole-repository certification |
