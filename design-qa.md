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
