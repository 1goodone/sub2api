export default {
  teams: {
    enabled: 'Teams fixed-cost recovery priority', cost: 'Cost per account (CNY)', hours: 'Paid window (hours)', conversion: 'CNY per billing unit', window: 'Window start',
    sources: { explicit: 'Explicit activation time', expiry: 'Count back from account expiry', first_usage: 'First usage (persisted)' },
    hint: 'Default: cost 50 per account, for 4 hours from its first use, in the same unit as user charges. Sum user charges across models and groups in that window. Prioritize recovery until revenue exceeds cost, then resume priority and economic ranking. Quality and concurrency limits remain active.',
    windowHint: 'Use 1 for the same currency; for USD billing, enter CNY per USD. Set explicit activation times below. Expired windows never roll forward automatically; first-use anchors are persisted; existing accounts initialize from the earliest retained record.',
    setStart: 'Also set activation time for selected Teams accounts', startHint: 'Only selected Teams accounts are changed. Resetting this time changes the paid window; use the actual activation time for this purchase.',
    recovering: 'Recovering cost', recovered: 'Cost exceeded', progress: 'CNY {gap} to recover · {minutes} minutes left', windowProfit: 'Window profit CNY {profit}'
  },

  profit: 'Recent profit / margin',
  economics: { usage: 'Usage records', rate: 'Rate estimate', unknown: 'Insufficient profit samples' },
  priority: 'Priority',
  batch: {
    typeOrder: 'Account type order', typeOrderHint: 'Drag or use the arrows to order Teams, Pro, Plus and API. Priorities are generated automatically; API rates ascend within the type. Applying writes account priorities; reloading infers the order from those values.', moveUp: 'Move {type} up', moveDown: 'Move {type} down', resetOrder: 'Reset Teams → Pro → Plus → API',

    title: 'Quick settings by account category', hint: 'Writes existing fields on physical accounts (excluding shadows), affecting scheduling even when priority scheduling is disabled. Load accounts to review Teams, Pro, Plus and API account-rate buckets. Billing rates are not modified.',
    group: 'Group ID (optional)', load: 'Load accounts and generate order', category: 'Category / rate', accounts: 'Accounts', current: 'Current priority / concurrency / load factor', priority: 'New priority',
    priorityHint: 'Lower priority numbers run first within an experience tier, before dynamic scores. Assign the same priority to categories that should compete on dynamic scores.',
    concurrency: 'Also set concurrency', loadFactor: 'Also set load factor', loadHint: 'Concurrency limits simultaneous requests. A higher load factor increases scheduling frequency. With concurrency 100 and load factor 10000, congestion and slots still use the actual concurrency limit. Unchecked fields retain their values.',
    preview: 'Update priority for {count} accounts; concurrency: {concurrency}; load factor: {loadFactor}.', keep: 'Keep existing', apply: 'Apply to selected accounts', empty: 'No accounts awaiting updates.', otherOAuth: 'OAuth · Other plans',
    loadError: 'Could not load accounts, or more than 10000 matched. Narrow by group and retry.', result: 'Updated {count} accounts.', partial: 'Some accounts were not updated. They remain in the list for retry.'
  },
  selectionWeight: 'Selection weight', boundGroups: 'Bound groups', explorationEligible: 'Eligible for limited exploration',
  title: 'Priority scheduling', description: 'Meet experience targets, then balance quality, latency, capacity and cost.', enabled: 'Enable priority scheduling',
  scopeNote: 'Applies to freely routed OpenAI text requests. Session binding, protocol preference, model permissions, rate limits and profit gates still apply. Images, video and other platforms keep their existing scheduling.',
  modes: { experience: 'Experience first', balanced: 'Balanced', profit: 'Profit first', custom: 'Custom' },
  strategy: 'Scheduling strategy', weights: 'Score weights', quality_weight: 'Quality', latency_weight: 'Latency', load_weight: 'Available capacity', cost_weight: 'Profit / cost',
  thresholds: 'Experience targets and observation window', target_ttft_ms: 'P90 first-token target (ms)', max_load_percent: 'Concurrency threshold (%)', min_quality_percent: 'Quality pass target (%)', window_minutes: 'Usage window (minutes)', min_samples: 'Minimum latency / profit samples', quality_max_age_hours: 'Quality freshness (hours)',
  groupIds: 'Group IDs', groupHint: 'Comma-separated; empty means all groups.', models: 'Models', modelHint: 'One exact requested model name per line; empty means all models.',
  rule: 'Order: targets met → insufficient data → targets missed. Within a tier, Teams accounts still recovering their paid-window cost come first; peers then honor explicit account priority and share traffic by real spare slots, quota headroom, bounded score preference and OAuth group contention. Safe unknown accounts get about a 10% exploration chance, never known degraded or recovery accounts. Movable sessions may spill over under material imbalance while keeping their binding.',
  costHint: 'Profit = user charges − estimated cost. Teams recovery uses its fixed paid window. Other accounts use the same-group, same-model base cost × the saved account cost multiplier (default 0.1). With following enabled, successful upstream probes save a new rate; turn it off to retain manual cost. Failed, expired, or unsupported probes preserve the saved value. Other OAuth accounts require usage samples; API accounts may fall back to the cost multiplier. Changes re-estimate recent profit without rewriting usage logs.',
  save: 'Save configuration', saving: 'Saving…', saved: 'Configuration saved', loading: 'Loading…', retry: 'Retry', error: 'Could not load or save. Please retry.', invalid: 'Check group IDs and parameter ranges',
  recent: 'Latest candidate scores', refresh: 'Refresh scores', empty: 'No scores yet. Eligible requests requiring free routing generate scores after enabling.', historyPending: 'History is not ready; existing scores remain active. Statistics refresh in the background every 30 seconds.',
  snapshotHint: 'Shows the latest pool on this instance, up to 100 accounts. Selection weights compare peers within the same protocol, subscription pool, tier and priority. Scores and row order are neither final selections nor fixed traffic shares; live slots, quota and group bindings affect distribution.',
  model: 'Model', group: 'Group', account: 'Account', score: 'Score', tier: 'Status', latency: 'P90 first token', load: 'Concurrency', rate: 'Cost rate', quality: 'Quality passed', samples: 'samples', unknown: 'Unknown',
  tiers: { eligible: 'Targets met', insufficient: 'Insufficient data', degraded: 'Targets missed' },
  reasons: { quality_below_target: 'Quality below target', quality_unknown: 'No fresh quality result', latency_above_target: 'Latency above target', latency_insufficient: 'Insufficient latency samples', busy: 'High concurrency or queueing', load_unknown: 'Concurrency unknown', cost_unknown: 'Cost unknown', recent_errors: 'Elevated recent errors', historical_loss: 'Recent charges below theoretical cost', profit_insufficient: 'Insufficient profit samples', teams_recovery: 'Paid-window revenue has not exceeded cost', teams_window_unavailable: 'Teams paid window missing or inactive' }
}
