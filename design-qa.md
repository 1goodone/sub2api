# Official Usage Details Design QA

- source visual truth path: `C:/Users/Even/AppData/Local/Temp/codex-clipboard-dea53e31-babc-4771-a89f-0bee9d5c11a8.png`
- implementation screenshot path: `C:/Users/Even/.codex/visualizations/2026/09/30/01a0f0c4-e290-7b81-85ce-b2db36fd8d8c/official-usage-desktop-final.png`
- comparison image path: `C:/Users/Even/.codex/visualizations/2026/09/30/01a0f0c4-e290-7b81-85ce-b2db36fd8d8c/official-usage-comparison.png`
- error-state screenshot path: `C:/Users/Even/.codex/visualizations/2026/09/30/01a0f0c4-e290-7b81-85ce-b2db36fd8d8c/official-usage-error.png`
- mobile screenshot path: `C:/Users/Even/.codex/visualizations/2026/09/30/01a0f0c4-e290-7b81-85ce-b2db36fd8d8c/official-usage-mobile-fixed.png`
- viewport: desktop `1200 x 1200`; mobile `390 x 844`
- pixels and density: source `1200 x 1200`; desktop implementation `1200 x 1200`; mobile implementation `390 x 844`; CSS viewport and image pixels were 1:1 at device scale factor 1, so no density normalization was required
- state: Chinese locale, light theme, OpenAI OAuth parent account, successful official settlement response; separate unauthorized upstream state

## Full-view comparison evidence

- The implementation preserves the source hierarchy: account context, official/local tab switch, range control, refresh, official totals, cycle estimate, daily cost bars, client split, and model/speed split.
- The layout intentionally follows the existing Sub2API account statistics modal instead of copying the reference application's outer navigation and six-card grid.
- Settled daily bars and official cost use green; upstream authorization/service errors use a red semantic panel; unsettled data remains amber.

## Focused region comparison evidence

- Header and controls: verified account-name truncation, range selection, refresh interaction, and official-tab lazy load.
- Summary and chart: verified cost/token/turn cards, cycle progress, daily bar scaling, token and turn labels, and green settlement state.
- Responsive layout: verified the modal at `390 x 844`; `document.body.scrollWidth === document.body.clientWidth` and `.modal-body.scrollWidth === .modal-body.clientWidth`.
- Error state: verified the unauthorized state renders a red alert with a sanitized user-facing message.
- Console: no warning or error entries during final desktop interaction.

## Findings

- No actionable P0/P1/P2 differences remain.
- P3 accepted difference: the implementation is denser than the reference and uses the product's existing modal title, cards, tokens, and spacing. This is intentional adaptation, not visual drift.

## Comparison history

1. Initial mobile capture found a P2 horizontal overflow in the account header and narrow daily rows.
2. Fixed the header with `min-w-0`, truncation, and shrink constraints; changed daily rows to responsive `minmax(0, 1fr)` columns.
3. Post-fix mobile capture confirmed no page or modal horizontal overflow.

## Primary interactions tested

- Open official settlement tab.
- Switch range from 30 days to 7 days and confirm selected styling.
- Trigger manual refresh.
- Render successful, unauthorized, desktop, and mobile states.

final result: passed

---

# Orbitix-inspired Home and Integrated Console QA — 2026-10-11

## Scope and acceptance

Adapt the reference at https://orbitix.site/home to the existing Sub2API brand. Support readable light and dark themes across the default home, authentication layout and console. Use a continuous edge-aligned sidebar/header; remove decorative console motion while preserving loading feedback. Preserve custom-home/compact-home precedence, registration and feature flags, authentication and routing.

Acceptance requires a successful production build/typecheck, lint, relevant regression tests, real desktop/mobile screenshots, theme/navigation/form checks, and evidence that the console does not load the homepage particle bundle. Live API latency is outside this frontend-only verification.

## Evidence

Artifacts are under `output/orbitix-design-review/` (local, ignored output):

- Reference: `01-home-desktop.jpg`, `06-login-desktop.jpg`, `07-home-mobile.jpg`, `reference-overview.jpg`.
- Implementation comparison: `implementation-overview.jpg`; home: `home-{light,dark}-desktop.jpg`, `home-{light,dark}-mobile.jpg`, `home-{light,dark}-full.jpg`.
- Authentication: `login-{light,dark}-desktop.jpg`, `login-dark-mobile.jpg`.
- Console: `dashboard-{light,dark}-desktop.jpg`, `keys-{light,dark}-desktop.jpg`, `keys-light-mobile.jpg`, `keys-light-tablet.jpg`.
- Motion/locale: `home-reduced-motion-tablet.jpg`, `home-english-mobile.jpg`.
- Verification logs: `build.log`, `lint.log`, `final-focused-tests.log`, `tests.log`, `baseline-tests.log`.

Desktop CSS viewport: 1440 × 900; phone: 390 × 844 and 320 × 740; tablet: 768 × 1024. Browser screenshots sometimes exclude scrollbar pixels; the overview normalizes each image proportionally to a 720 × 450 tile. No pixel-exact clone claim is made: light mode and a non-floating console are intentional user-requested adaptations.

## Results

- Production build (including vue-tsc and locale completeness): passed. ESLint and `git diff --check`: passed.
- Final relevant regression run: 27 files, 221 tests passed. Includes home override precedence, register/login/admin destinations, reduced motion, animation pause/resume, configured API endpoint, auth flows, sidebar, dialogs, tables and locales.
- Full suite: 439 files passed; 3 files / 10 tests failed; 9 skipped. The same 10 failures reproduce against an isolated, unmodified HEAD frontend using the same installed dependencies: AccountUsageCell has 8 missing-helper failures, ModelWhitelistSelector has 1 expectation failure, IQTestModal has 1 model-selection expectation failure. These are not repaired by this UI change.
- Both themes rendered with no page-level horizontal overflow at checked breakpoints. Mobile navigation opens at full width even after desktop collapse; hidden navigation is inert; Escape closes it and returns focus to the menu toggle. Collapsed links retain accessible names.
- Local fixture login reaches the real dashboard route. Key table, horizontal table scrolling, create-dialog opening/cancel, theme change, home anchors, FAQ, copy feedback, and English language switch work. No production account or data was used.
- Actual console geometry: sidebar at x=0/y=0, width 256px or 72px; header at y=0, height 64px. Shell border radius 0; shadow none; sidebar transition 0s. Modal backdrop blur and modal transitions removed.
- A fresh production login/dashboard session requested no `vendor-orbit`, `OrbitScene`, `HomeView` or Manrope assets. The two dashboard canvases are charts. Particle code is confined to the async homepage component; its vendor chunk is approximately 174kB gzip.
- Browser Event Timing samples on this machine: collapse 24ms, expand 24ms, theme toggle 40ms (input delay about 0.2ms; processing 1.4ms, 0.8ms, 19.6ms). Three samples, no throttling, local fixtures: these are not field INP measurements or proof of improved network latency.
- Homepage animation stops when paused, offscreen, or when `prefers-reduced-motion: reduce` is emulated; reduced-motion scrolling is immediate. Scene is removed after navigation. Temporary browser emulation and timing observers are restored/removed after checks.
- Production preview uses nonce-based script CSP. The early theme script carries the backend's existing nonce placeholder; no unsafe script allowance was added. Browser console contains no errors in the final production login/dashboard/keys/home checks.
- Calculated solid-color contrast: light primary button 6.05:1; dark primary button 10.71:1; home muted body text 5.66:1 light / 8.46:1 dark. Keyboard focus is visible. This is not a complete screen-reader audit of every existing product page.

## Findings and fixes

Fixed during QA: dark CSS scoping; theme-click argument typing; missing CSP nonce; missing mobile section menu; collapsed desktop navigation carrying into mobile; unlabeled collapsed links; old fixed-table-column colors; missing mobile page title; modal child transition; configured gateway address handling. Incomplete local fixtures were corrected without changing production API behavior.

The reference's marketing composition is adapted through oversized real text, a particle orbit, teal emphasis and separated content chapters. Internal pages use opaque surfaces, thin dividers and immediate interactions. Existing endpoint/auth settings, data fetching and authorization remain the source of behavior.

## Quality self-assessment

| Dimension | Score / 10 | Evidence or remaining limitation |
| --- | --- | --- |
| Functionality | 9.5 | 221 relevant tests and real route/control checks |
| Code | 9.5 | Typecheck, lint, production build; existing Vue/Pinia/router patterns retained |
| UI | 9.5 | Light/dark desktop/mobile/tablet screenshots; corrected fixed-column surfaces |
| UX | 9.5 | Immediate shell/menu/dialog actions, usable mobile navigation, visible focus |
| Art direction | 9.5 | Coherent typography, orbit composition and two deliberate theme palettes |
| Performance | 9.3 | Bundle isolation and local timings verified; real-device/production p75 INP and API latency remain to measure |
| Security | 9.5 | Existing auth/URL sanitization retained, CSP nonce verified; local synthetic data only |
| Overall | 9.4 | Deliverable in the tested scope; no high-quality or full-site performance certification claimed |

Remaining follow-up: measure production interaction latency and representative low-end mobile rendering after integration, and track the 10 pre-existing test failures separately. Existing large application chunks still produce Vite size warnings; this change isolates its added visual dependency instead of refactoring unrelated modules.

final result: passed for the scoped UI acceptance criteria; full-repository test suite remains blocked by the independently reproduced pre-existing failures.

## Claude implementation continuation — 2026-10-11

This entry supersedes the earlier homepage evidence. Claude replaced the prior renderer with a native WebGL universe; the console implementation was retained. Scope: preserve that design, repair observed blockers, and provide a local interactive preview. Acceptance: build and focused tests pass; mouse parallax, chapter scrolling, both themes, mobile navigation and motion controls work in the browser. Real mobile GPU performance remains outside this desktop preview's validation.

Repairs: await Vue's DOM flush in the scene test and clean up mounts; initialize parallax despite NaN sentinel values; pause CSS nebula/chart animations; bind the light palette to the actual site theme; apply split-letter styles across Vue's component scope; restore intended spacing and button contrast; fit mobile navigation and separate the mobile hero title from body copy; start sticky chapter content at the chapter boundary. No new dependency was added.

Verification:
- 26 focused tests passed, including mouse-follow/reset regression and 21 existing home-mode checks. ESLint, vue-tsc, locale validation, production build and final CSS-only Vite rebuild passed; existing bundle-size warnings remain.
- Browser: dark/light at 1440 × 900, mobile checks at 390 × 844 and 320 × 740. At 320px the hero copy starts at y=390.9, after the title's y=324.4 bottom; no page horizontal overflow. Mobile menu opens and Escape closes it.
- Light theme resolves to rgb(238,243,246); split letters have actual inline-block layout and 0.8s/1.1s transitions. Mouse movement produces distinct depth-dependent transforms on model labels.
- Chapter navigation now places capability cards at y=394 instead of below the viewport; scrolling moves the horizontal track. Manual pause stops the renderer and pauses the nebula. Emulated reduced motion stops the renderer and removes nebula animation; emulation and viewport override were restored.
- Fresh dashboard navigation requested no HomeView/OrbitScene/universe/formations assets and mounted no particle scene. Final homepage console had no warnings or errors. Preview uses local synthetic API fixtures, not production data.
- Evidence: output/orbitix-design-review/claude-continuation-{dark,light,mobile,caps}.png; corresponding tests, lint, build and final-vite logs; console isolation JSON and a desktop interaction performance sample. Earlier renderer timing figures do not describe this version.

| Dimension | Score / 10 | Scope / remaining improvement |
| --- | --- | --- |
| Functionality | 9.5 | Focused tests and browser interactions pass |
| Code | 9.5 | Minimal fixes, cleanup regression, typecheck/lint/build pass |
| UI / UX | 9.3 | Both palettes and mobile navigation verified; smallest-screen text wrapping can be further polished |
| Art direction | 9.4 | Particle formations, glass wordmark and scroll scenes work; density and animation pacing need user preference review |
| Performance | 9.3 | Native renderer split from console and pause verified; real low-end mobile GPU/FPS not measured |
| Security | N/A | This continuation changes local visual rendering/tests, not access control or data handling; no security audit claimed |
| Overall | 9.4 | Deliverable local preview, not production performance certification |

Homepage branding follow-up: requested Sub2API display name is mapped to 小羊咩咩 in the homepage's existing name source, so its header, accessible heading, WebGL texture, particle finale and footer share the new name. Other configured site names and global console branding remain configurable. Existing 26 focused tests pass.
