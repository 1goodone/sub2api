import type { ScheduledTestPlan } from '@/types'
import { apiClient } from '../client'
import type { ModelBillingConfig } from '@/utils/modelBilling'
import type { ExcelBPSDefaults } from '@/utils/excelBPSDefaults'
import type { OAuthModelMappingRule } from '@/utils/oauthModelMappings'
export type OAuthInitialQualityRule = Pick<ScheduledTestPlan, 'model_id' | 'cron_expression' | 'enabled' | 'max_results' | 'pelican_config'>
export interface OAuthInitialAccountSettings {
  rate_multiplier?: number | null
  cost_multiplier?: number | null
  proxy_id?: number | null
  auto_pause_on_expired?: boolean | null
  openai_passthrough?: boolean | null
  openai_flatten_namespaces?: boolean | null
  openai_long_context_billing?: boolean | null
  codex_cli_only?: boolean | null
  codex_cli_only_app_server?: boolean | null
  codex_fingerprint_mode?: string | null
  openai_compact_mode?: string | null
  openai_request_timezone?: string | null
  slot_reservation_enabled?: boolean | null
  slot_reservation_seconds?: number | null
  openai_ws_mode?: string | null
  intercept_warmup?: boolean | null
}
export interface AutoConfig {
 quality_rule?: OAuthInitialQualityRule | null
 model_mappings?: OAuthModelMappingRule[] | null
 model_billing?: ModelBillingConfig
 excel_bps?: ExcelBPSDefaults
 account_settings?: OAuthInitialAccountSettings
 enabled: boolean
 platform: string
 priority: number
 load_factor: number
 concurrency: number
 group_ids: number[]
 upgrade_enabled: boolean
 upgrade_group_ids: number[]
 successes_per_step: number
 upgrade_step: number
 max_concurrency: number
 cooldown_seconds: number
 revision: string
 runtime_blocked?: boolean
}
export async function getAutoConfig(): Promise<AutoConfig> { return (await apiClient.get('/admin/account-ops/auto-config')).data }
export async function saveAutoConfig(config: AutoConfig): Promise<AutoConfig> { return (await apiClient.put('/admin/account-ops/auto-config', config)).data }

export type AutoConfigEventKind = 'config_saved' | 'initial_applied' | 'concurrency_upgraded' | 'failure_cooldown'
export interface AutoConfigEvent {
  id: number
  account_id: number
  account_name: string
  platform: string
  kind: AutoConfigEventKind
  created_at: string
  details: {
    model_mapping?: Record<string, string>
    config?: AutoConfig
    priority: number
    load_factor: number
    concurrency: number
    previous_concurrency: number
    group_ids?: number[]
    cooldown_seconds: number
  }
}
export interface AutoConfigEventsPage {
  items: AutoConfigEvent[]
  has_more: boolean
}
export async function getAutoConfigEvents(params: { before?: number; kind?: AutoConfigEventKind | ''; limit?: number } = {}): Promise<AutoConfigEventsPage> {
  return (await apiClient.get('/admin/account-ops/auto-config/events', { params })).data
}
