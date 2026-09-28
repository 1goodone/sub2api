<template>
  <AppLayout>
    <div class="guard-v2">
      <SmartOpsNav />
      <header class="page-heading">
        <div>
          <p class="eyebrow">{{ t('accountOps.smartTitle') }}</p>
          <h2>{{ t('tokenGuardV2.title') }}</h2>
          <p class="subtitle">{{ t('tokenGuardV2.description') }}</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading" @click="load()">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t('tokenGuardV2.refresh') }}
          </button>
          <button class="btn btn-primary" @click="openCreate">{{ t('tokenGuardV2.addAccount') }}</button>
        </div>
      </header>

      <p v-if="error && !editorOpen" role="alert" class="error-banner">{{ error }}</p>
      <p v-if="notice" role="status" class="success-banner">{{ notice }}</p>

      <section class="summary-grid">
        <article class="summary-card"><span>{{ t('tokenGuardV2.monitored') }}</span><strong>{{ accounts.length }}</strong><small>{{ t('tokenGuardV2.intervalHint', { minutes: Math.round(status.probe_interval_seconds / 60) }) }}</small></article>
        <article class="summary-card"><span>{{ t('tokenGuardV2.healthy') }}</span><strong>{{ healthyCount }}</strong><small>{{ t('tokenGuardV2.enabledCount', { count: enabledCount }) }}</small></article>
        <article class="summary-card"><span>{{ t('tokenGuardV2.needsAttention') }}</span><strong>{{ issueCount }}</strong><small>{{ t('tokenGuardV2.failureThreshold', { count: status.fail_streak_threshold }) }}</small></article>
        <article class="summary-card"><span>{{ t('tokenGuardV2.activeTasks') }}</span><strong>{{ activeTaskCount }}</strong><small>{{ t('tokenGuardV2.workerHint') }}</small></article>
      </section>

      <section class="table-card">
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>{{ t('tokenGuardV2.account') }}</th>
                <th>{{ t('tokenGuardV2.loginMode') }}</th>
                <th>{{ t('tokenGuardV2.configuration') }}</th>
                <th>{{ t('tokenGuardV2.inspection') }}</th>
                <th>{{ t('tokenGuardV2.failureStreak') }}</th>
                <th>{{ t('tokenGuardV2.lastInspection') }}</th>
                <th>{{ t('tokenGuardV2.lastRelogin') }}</th>
                <th>{{ t('tokenGuardV2.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in accounts" :key="item.account_id">
                <td><strong>{{ item.account_name || `#${item.account_id}` }}</strong><small>#{{ item.account_id }} · {{ item.account_status }}</small></td>
                <td>{{ modeLabel(item.login_config?.credential_mode) }}<small>{{ item.login_config?.login_email || '-' }}</small></td>
                <td><span class="badge" :class="item.login_config?.configured ? 'ok' : 'danger'">{{ item.login_config?.configured ? t('tokenGuardV2.configured') : t('tokenGuardV2.incomplete') }}</span><small>{{ configurationDetail(item) }}</small></td>
                <td><span class="badge" :class="probeClass(item.probe_state)">{{ t(`tokenGuardV2.probeStates.${item.probe_state || 'pending'}`) }}</span><small>{{ item.blocked_reason || item.probe_detail || '-' }}</small></td>
                <td class="tabular-nums">{{ item.fail_streak }}</td>
                <td>{{ date(item.last_probe_at) }}<small v-if="item.enabled">{{ t('tokenGuardV2.next') }} {{ date(item.next_probe_at) }}</small><small v-else>{{ t('tokenGuardV2.paused') }}</small></td>
                <td>{{ date(item.last_reauth_at) }}<small v-if="item.latest_task">{{ taskLabel(item.latest_task) }}</small></td>
                <td>
                  <div class="actions">
                    <button class="link-btn" @click="openEdit(item)">{{ t('tokenGuardV2.edit') }}</button>
                    <button class="link-btn" :disabled="busyId === item.account_id" @click="probe(item)">{{ t('tokenGuardV2.inspectNow') }}</button>
                    <button class="link-btn" :disabled="busyId === item.account_id" @click="relogin(item)">{{ t('tokenGuardV2.reloginNow') }}</button>
                    <button class="link-btn" :disabled="busyId === item.account_id" @click="toggle(item)">{{ item.enabled ? t('tokenGuardV2.pause') : t('tokenGuardV2.resume') }}</button>
                    <button class="link-btn danger-text" :disabled="busyId === item.account_id" @click="remove(item)">{{ t('tokenGuardV2.remove') }}</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-if="!loading && !accounts.length" class="empty-state">
            <Icon name="shield" size="xl" />
            <h3>{{ t('tokenGuardV2.emptyTitle') }}</h3>
            <p>{{ t('tokenGuardV2.emptyHint') }}</p>
          </div>
        </div>
      </section>

      <BaseDialog :show="editorOpen" :title="editing ? t('tokenGuardV2.editTitle') : t('tokenGuardV2.addTitle')" width="wide" @close="closeEditor">
        <form id="token-guard-v2-editor" class="editor-form" @submit.prevent="save">
          <p v-if="error" role="alert" class="error-banner">{{ error }}</p>
          <label class="field-label">{{ t('tokenGuardV2.account') }}
            <select v-model.number="draft.account_id" class="input w-full" :disabled="Boolean(editing)" required @change="fillLoginEmail">
              <option :value="0" disabled>{{ t('tokenGuardV2.selectAccount') }}</option>
              <option v-for="account in selectableAccounts" :key="account.id" :value="account.id">{{ account.name }} (#{{ account.id }})</option>
            </select>
          </label>
          <label class="field-label">{{ t('tokenGuardV2.loginEmail') }}<input v-model.trim="draft.login_email" class="input w-full" type="email" required /></label>
          <label class="field-label">{{ t('tokenGuardV2.loginProxy') }}
            <select v-model="proxyChoice" class="input w-full">
              <option value="account">{{ t('tokenGuardV2.accountProxyDefault') }}</option>
              <option value="mihomo">{{ t('tokenGuardV2.mihomoManagedPool') }}</option>
              <option v-for="proxy in proxies" :key="proxy.id" :value="`proxy:${proxy.id}`">{{ proxyOptionLabel(proxy) }}</option>
            </select>
            <small class="field-hint">{{ t('tokenGuardV2.loginProxyHint') }}</small>
          </label>
          <fieldset class="mode-grid">
            <legend>{{ t('tokenGuardV2.loginMode') }}</legend>
            <label class="mode-option"><input v-model="draft.credential_mode" type="radio" value="password_totp" /><span><strong>{{ t('tokenGuardV2.passwordMode') }}</strong><small>{{ t('tokenGuardV2.passwordModeHint') }}</small></span></label>
            <label class="mode-option"><input v-model="draft.credential_mode" type="radio" value="email_otp_url" /><span><strong>{{ t('tokenGuardV2.mailboxMode') }}</strong><small>{{ t('tokenGuardV2.mailboxModeHint') }}</small></span></label>
          </fieldset>

          <template v-if="draft.credential_mode === 'password_totp'">
            <label class="field-label">{{ t('tokenGuardV2.password') }}<input v-model="draft.password" class="input w-full" type="password" :placeholder="editing?.login_config?.password_configured ? t('tokenGuardV2.keepSecret') : ''" :required="!editing?.login_config?.password_configured" autocomplete="new-password" /></label>
            <label class="field-label">{{ t('tokenGuardV2.totpSecret') }}<input v-model.trim="draft.totp_secret" class="input w-full" type="password" :placeholder="editing?.login_config?.totp_configured ? t('tokenGuardV2.keepSecret') : t('tokenGuardV2.optional')" autocomplete="off" /></label>
            <label v-if="editing?.login_config?.totp_configured" class="check-row"><input v-model="draft.clear_totp" type="checkbox" />{{ t('tokenGuardV2.clearTotp') }}</label>
          </template>
          <label v-else class="field-label">{{ t('tokenGuardV2.otpUrl') }}<input v-model.trim="draft.otp_url" class="input w-full" type="url" :placeholder="editing?.login_config?.otp_url_masked ? `${editing.login_config.otp_url_masked} · ${t('tokenGuardV2.keepSecret')}` : 'https://mail.example.com/latest'" :required="editing?.login_config?.credential_mode !== 'email_otp_url' || !editing?.login_config?.otp_url_masked" /></label>

          <div class="toggle-grid">
            <label class="toggle-row"><input v-model="draft.enabled" type="checkbox" /><span><strong>{{ t('tokenGuardV2.autoInspect') }}</strong><small>{{ t('tokenGuardV2.autoInspectHint') }}</small></span></label>
            <label class="toggle-row"><input v-model="draft.auto_relogin_enabled" type="checkbox" /><span><strong>{{ t('tokenGuardV2.autoRelogin') }}</strong><small>{{ t('tokenGuardV2.autoReloginHint') }}</small></span></label>
          </div>
        </form>
        <template #footer>
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeEditor">{{ t('tokenGuardV2.cancel') }}</button>
          <button type="submit" form="token-guard-v2-editor" class="btn btn-primary" :disabled="saving || !draft.account_id">{{ saving ? t('tokenGuardV2.saving') : t('tokenGuardV2.save') }}</button>
        </template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import SmartOpsNav from '@/components/admin/operations/SmartOpsNav.vue'
import Icon from '@/components/icons/Icon.vue'
import { BaseDialog } from '@/components/common'
import { list as listAccounts } from '@/api/admin/accounts'
import { getAllWithCount as listProxies } from '@/api/admin/proxies'
import type { AccountListItem, Proxy } from '@/types'
import {
  createTokenGuardV2Account,
  deleteTokenGuardV2Account,
  listTokenGuardV2Accounts,
  probeTokenGuardV2Account,
  reloginTokenGuardV2Account,
  updateTokenGuardV2Account,
  type SaveTokenGuardV2Account,
  type TokenGuardV2Account,
  type TokenGuardV2CredentialMode,
  type TokenGuardV2ProxySource,
  type TokenGuardV2Status,
  type TokenGuardV2Task
} from '@/api/admin/accountTokenGuardV2'

const { t } = useI18n()
const emptyStatus = (): TokenGuardV2Status => ({ accounts: [], probe_interval_seconds: 1800, fail_streak_threshold: 2 })
const status = reactive<TokenGuardV2Status>(emptyStatus())
const candidates = ref<AccountListItem[]>([])
const proxies = ref<Proxy[]>([])
const loading = ref(false), saving = ref(false), busyId = ref(0), editorOpen = ref(false)
const editing = ref<TokenGuardV2Account | null>(null)
const error = ref(''), notice = ref('')
let timer: ReturnType<typeof setInterval> | undefined

const blankDraft = (): SaveTokenGuardV2Account => ({
  account_id: 0,
  login_email: '',
  credential_mode: 'password_totp',
  proxy_source: 'account',
  proxy_id: null,
  password: '',
  totp_secret: '',
  otp_url: '',
  clear_totp: false,
  enabled: true,
  auto_relogin_enabled: true
})
const draft = reactive<SaveTokenGuardV2Account>(blankDraft())

const accounts = computed(() => status.accounts)
const enabledCount = computed(() => accounts.value.filter(item => item.enabled).length)
const healthyCount = computed(() => accounts.value.filter(item => item.probe_state === 'ok').length)
const issueCount = computed(() => accounts.value.filter(item => item.probe_state === 'auth' || item.blocked_reason).length)
const activeTaskCount = computed(() => accounts.value.filter(item => ['queued', 'running', 'callback_processing'].includes(item.latest_task?.status || '')).length)
const selectableAccounts = computed(() => {
  const monitored = new Set(accounts.value.map(item => item.account_id))
  return editing.value ? candidates.value : candidates.value.filter(item => !monitored.has(item.id))
})

const message = (value: unknown) => (value as { message?: string })?.message || t('tokenGuardV2.error')
const date = (value?: string) => value ? new Date(value).toLocaleString() : '-'
const modeLabel = (mode?: TokenGuardV2CredentialMode) => !mode ? '-' : mode === 'password_totp' ? t('tokenGuardV2.passwordMode') : t('tokenGuardV2.mailboxMode')
const proxyChoice = computed({
  get: () => draft.proxy_source === 'managed_proxy' && draft.proxy_id ? `proxy:${draft.proxy_id}` : draft.proxy_source,
  set: (value: string) => {
    if (value.startsWith('proxy:')) {
      draft.proxy_source = 'managed_proxy'
      draft.proxy_id = Number(value.slice(6))
      return
    }
    draft.proxy_source = value as TokenGuardV2ProxySource
    draft.proxy_id = null
  }
})
const proxyOptionLabel = (proxy: Proxy) => `${proxy.protocol.toUpperCase()} · ${proxy.name} · ${proxy.host}:${proxy.port}`
const proxyLabel = (source: TokenGuardV2ProxySource = 'account', proxyId?: number) => {
  if (source === 'mihomo') return t('tokenGuardV2.mihomoManagedPool')
  if (source !== 'managed_proxy' || !proxyId) return t('tokenGuardV2.accountProxyDefault')
  const proxy = proxies.value.find(item => item.id === proxyId)
  return proxy ? proxyOptionLabel(proxy) : `#${proxyId}`
}
const probeClass = (state: string) => state === 'ok' ? 'ok' : state === 'auth' ? 'danger' : ''
const taskLabel = (task: TokenGuardV2Task) => task.error ? `${task.status}: ${task.error}` : `${task.status} · ${task.stage}`
const configurationDetail = (item: TokenGuardV2Account) => {
  const config = item.login_config
  if (!config) return '-'
  const credentials = config.credential_mode === 'email_otp_url'
    ? config.otp_url_masked || '-'
    : [config.password_configured ? t('tokenGuardV2.passwordSaved') : '', config.totp_configured ? t('tokenGuardV2.totpSaved') : ''].filter(Boolean).join(' · ') || '-'
  return `${credentials} · ${t('tokenGuardV2.loginProxy')}: ${proxyLabel(config.proxy_source, config.proxy_id)}`
}
const accountEmail = (account?: AccountListItem) => {
  const email = account?.extra?.email_address || account?.extra?.email || account?.credentials?.email
  return typeof email === 'string' ? email.trim() : ''
}

function resetDraft() { Object.assign(draft, blankDraft()) }
function openCreate() { error.value = ''; notice.value = ''; editing.value = null; resetDraft(); editorOpen.value = true }
function fillLoginEmail() { draft.login_email = accountEmail(candidates.value.find(item => item.id === draft.account_id)) }
function openEdit(item: TokenGuardV2Account) {
  error.value = ''; notice.value = ''
  editing.value = item
  Object.assign(draft, blankDraft(), {
    account_id: item.account_id,
    login_email: item.login_config?.login_email || '',
    credential_mode: item.login_config?.credential_mode || 'password_totp',
    proxy_source: item.login_config?.proxy_source || (item.login_config?.proxy_id ? 'managed_proxy' : 'account'),
    proxy_id: item.login_config?.proxy_id ?? null,
    enabled: item.enabled,
    auto_relogin_enabled: item.auto_relogin_enabled
  })
  editorOpen.value = true
}
function closeEditor() { if (!saving.value) editorOpen.value = false }

async function load(silent = false) {
  if (loading.value) return
  loading.value = true
  if (!silent) error.value = ''
  try {
    const [guard, accountPage, managedProxies] = await Promise.all([
      listTokenGuardV2Accounts(),
      listAccounts(1, 200, { platform: 'openai', type: 'oauth', lite: '1' }),
      listProxies()
    ])
    Object.assign(status, guard)
    candidates.value = accountPage.items.filter(item => !item.parent_account_id)
    proxies.value = managedProxies
  } catch (value) {
    if (!silent) error.value = message(value)
  } finally {
    loading.value = false
  }
}

async function save() {
  if (saving.value || !draft.account_id) return
  saving.value = true; error.value = ''; notice.value = ''
  try {
    const payload = { ...draft }
    if (editing.value) await updateTokenGuardV2Account(draft.account_id!, payload)
    else await createTokenGuardV2Account(payload)
    notice.value = t('tokenGuardV2.saved')
    editorOpen.value = false
    await load(true)
  } catch (value) {
    error.value = message(value)
  } finally {
    saving.value = false
  }
}

async function act(item: TokenGuardV2Account, action: () => Promise<unknown>, successKey: string) {
  if (busyId.value) return
  busyId.value = item.account_id; error.value = ''; notice.value = ''
  try {
    await action()
    notice.value = t(successKey, { account: item.account_name })
    await load(true)
  } catch (value) {
    error.value = message(value)
  } finally {
    busyId.value = 0
  }
}

const probe = (item: TokenGuardV2Account) => act(item, () => probeTokenGuardV2Account(item.account_id), 'tokenGuardV2.inspectionQueued')
const relogin = (item: TokenGuardV2Account) => act(item, () => reloginTokenGuardV2Account(item.account_id), 'tokenGuardV2.reloginQueued')
const toggle = (item: TokenGuardV2Account) => act(item, () => updateTokenGuardV2Account(item.account_id, {
  login_email: item.login_config?.login_email || '',
  credential_mode: item.login_config?.credential_mode || 'password_totp',
  proxy_source: item.login_config?.proxy_source || (item.login_config?.proxy_id ? 'managed_proxy' : 'account'),
  proxy_id: item.login_config?.proxy_id ?? null,
  enabled: !item.enabled,
  auto_relogin_enabled: item.auto_relogin_enabled
}), item.enabled ? 'tokenGuardV2.pausedNotice' : 'tokenGuardV2.resumedNotice')
const remove = async (item: TokenGuardV2Account) => {
  if (!window.confirm(t('tokenGuardV2.removeConfirm', { account: item.account_name }))) return
  await act(item, () => deleteTokenGuardV2Account(item.account_id), 'tokenGuardV2.removedNotice')
}

onMounted(() => {
  void load()
  timer = setInterval(() => { if (document.visibilityState === 'visible' && !editorOpen.value) void load(true) }, 30_000)
})
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<style scoped>
.guard-v2 { max-width: 1660px; margin: auto; @apply text-gray-900 dark:text-gray-100; }
.page-heading { @apply mb-6 flex flex-wrap items-center justify-between gap-4; }
.eyebrow { @apply mb-1 text-[11px] font-semibold tracking-widest text-primary-600; }
.page-heading h2 { @apply text-2xl font-semibold tracking-tight; }
.subtitle { @apply mt-2 max-w-3xl text-sm leading-6 text-gray-500 dark:text-gray-400; }
.summary-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(190px,1fr)); gap:20px; @apply mb-5; }
.summary-card,.table-card { @apply overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900; }
.summary-card { @apply p-5; }
.summary-card span { @apply block text-xs font-medium text-gray-400; }
.summary-card strong { @apply mt-2 block text-2xl font-semibold tabular-nums; }
.summary-card small { @apply mt-2 block truncate text-xs text-gray-400; }
.table-wrap { @apply overflow-auto overscroll-contain; scrollbar-gutter:stable; }
table { min-width:1180px; @apply w-full text-left text-xs; }
thead { @apply sticky top-0 z-10 bg-white text-gray-400 dark:bg-dark-900; }
th { @apply whitespace-nowrap px-4 py-3 font-medium; }
td { @apply border-b border-gray-100 px-4 py-4 align-top dark:border-dark-800; }
td strong { @apply block max-w-52 truncate text-[13px] font-medium; }
td small { @apply mt-1.5 block max-w-64 text-[10px] leading-4 text-gray-400; }
tbody tr:hover { @apply bg-gray-50/70 dark:bg-dark-800/50; }
.badge { @apply inline-flex rounded-md bg-amber-50 px-2 py-1 text-[11px] font-medium text-amber-700 dark:bg-amber-950/30 dark:text-amber-300; }
.badge.ok { @apply bg-emerald-50 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300; }
.badge.danger { @apply bg-red-50 text-red-600 dark:bg-red-950/30 dark:text-red-300; }
.actions { @apply flex max-w-64 flex-wrap gap-1.5; }
.link-btn { @apply rounded-lg border border-gray-200 px-2.5 py-1 text-[11px] font-medium text-primary-600 transition-colors hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:hover:bg-dark-800; }
.danger-text { @apply text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30; }
.empty-state { @apply flex min-h-72 flex-col items-center justify-center gap-3 p-8 text-center text-gray-400; }
.empty-state h3 { @apply text-sm font-medium text-gray-600 dark:text-gray-300; }
.empty-state p { @apply max-w-md text-xs leading-5; }
.editor-form { @apply space-y-5; }
.field-label { @apply block text-xs font-medium text-gray-600 dark:text-gray-300; }
.field-label .input { @apply mt-2; }
.field-hint { @apply mt-1.5 block text-[11px] font-normal leading-4 text-gray-400; }
.mode-grid { @apply grid gap-3 sm:grid-cols-2; }
.mode-grid legend { @apply col-span-full mb-1 text-xs font-medium text-gray-600 dark:text-gray-300; }
.mode-option,.toggle-row { @apply flex items-start gap-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700; }
.mode-option:has(input:checked),.toggle-row:has(input:checked) { @apply border-primary-300 bg-primary-50/40 dark:border-primary-800 dark:bg-primary-950/20; }
.mode-option input,.toggle-row input,.check-row input { @apply mt-1 rounded text-primary-600; }
.mode-option strong,.toggle-row strong { @apply block text-sm font-medium; }
.mode-option small,.toggle-row small { @apply mt-1 block text-xs leading-5 text-gray-400; }
.toggle-grid { @apply grid gap-3 sm:grid-cols-2; }
.check-row { @apply flex items-center gap-2 text-xs text-gray-500; }
.error-banner { @apply mb-4 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300; }
.success-banner { @apply mb-4 rounded-xl bg-emerald-50 p-3 text-sm text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300; }
</style>
