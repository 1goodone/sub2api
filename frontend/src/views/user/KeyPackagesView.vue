<template>
  <AppLayout>
    <div class="package-workspace">
      <header class="package-visual-header">
        <img
          src="/illustrations/package-data-fabric.webp"
          alt=""
          aria-hidden="true"
          width="1440"
          height="960"
          fetchpriority="high"
          decoding="async"
          class="package-visual-image"
        />
        <div class="package-visual-scrim" aria-hidden="true"></div>
        <div class="package-visual-content">
          <div class="package-visual-copy">
            <h1>{{ t('keyPackages.title') }}</h1>
            <p>{{ t('keyPackages.description') }}</p>
          </div>
          <div class="package-visual-actions">
            <span class="visual-chip">
              <Icon name="calendar" size="sm" />
              {{ t('keyPackages.thirtyDays') }}
            </span>
            <span v-if="totalBoundKeys > 0" class="visual-chip">
              <Icon name="key" size="sm" />
              {{ t('keyPackages.totalBoundKeys', { count: totalBoundKeys }) }}
            </span>
            <button
              type="button"
              class="visual-icon-action"
              :disabled="packagesLoading"
              :aria-label="t('common.refresh')"
              :title="t('common.refresh')"
              @click="loadPage()"
            >
              <Icon name="refresh" size="md" :class="packagesLoading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </header>

      <div
        v-if="packagesError"
        class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
        role="alert"
      >
        <span>{{ packagesError }}</span>
        <button type="button" class="btn btn-secondary btn-sm" @click="loadPage()">
          {{ t('keyPackages.retry') }}
        </button>
      </div>

      <section v-if="!activePackage" class="plan-section" :aria-label="t('keyPackages.choosePlan')">
        <div class="plan-section-header">
          <div class="min-w-0">
            <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('keyPackages.choosePlan') }}</h2>
            <p class="mt-1 max-w-3xl text-sm leading-5 text-gray-500 dark:text-gray-400">{{ t('keyPackages.planPeriodNotice') }}</p>
          </div>
        </div>

        <div v-if="packagesLoading && plans.length === 0" class="plan-grid" aria-busy="true">
          <div v-for="item in 5" :key="item" class="plan-card">
            <Skeleton width="45%" height="22px" />
            <Skeleton class="mt-3" width="72%" height="14px" />
            <Skeleton class="mt-7" width="58%" height="34px" />
            <Skeleton class="mt-8" width="100%" height="42px" />
          </div>
        </div>

        <div v-else class="plan-grid">
          <article
            v-for="(plan, index) in plans"
            :key="plan.id"
            class="plan-card"
            :class="[
              selectedPlanId === plan.id ? 'plan-card-selected' : '',
              plan.id === 'plus' ? 'plan-card-featured' : '',
            ]"
            :data-plan="plan.id"
            :data-test="`plan-card-${plan.id}`"
          >
            <span class="plan-accent" aria-hidden="true"></span>
            <div class="plan-card-meta">
              <span class="plan-sequence">{{ String(index + 1).padStart(2, '0') }}</span>
              <span class="plan-cycle-label">
                <Icon name="calendar" size="sm" />
                {{ t('keyPackages.thirtyDays') }}
              </span>
              <span v-if="plan.id === 'plus'" class="recommended-badge">{{ t('keyPackages.recommended') }}</span>
            </div>

            <div class="plan-copy">
              <h3 class="plan-name">{{ plan.name }}</h3>
              <p class="plan-description">{{ t(`keyPackages.plans.${plan.id}`) }}</p>
            </div>

            <div class="plan-price-row">
              <span class="plan-price" :class="plan.amount_usd >= 1000 ? 'plan-price-compact' : ''">{{ formatCurrency(plan.amount_usd) }}</span>
              <span class="plan-price-unit">/{{ t('keyPackages.monthUnit') }}</span>
            </div>

            <dl class="plan-limits">
              <div>
                <dt><span class="limit-dot" aria-hidden="true"></span>5h</dt>
                <dd>{{ formatLimit(calculateLimit(plan.amount_usd, 'month', 5)) }}</dd>
              </div>
              <div>
                <dt><span class="limit-dot" aria-hidden="true"></span>7d</dt>
                <dd>{{ formatLimit(calculateLimit(plan.amount_usd, 'month', 168)) }}</dd>
              </div>
            </dl>

            <button
              type="button"
              class="plan-select-button"
              :class="selectedPlanId === plan.id ? 'plan-select-button-selected' : ''"
              :aria-pressed="selectedPlanId === plan.id"
              :data-test="`select-plan-${plan.id}`"
              @click="choosePlan(plan.id)"
            >
              <span>{{ t('keyPackages.selectAndSetKey') }}</span>
              <Icon name="arrowRight" size="sm" />
            </button>
          </article>
        </div>
      </section>

      <section v-if="activePackage && selectedPlan && selectedPackage" class="package-detail-panel" :aria-label="t('keyPackages.keysTitle')">
          <div class="package-detail-header">
            <div class="flex min-w-0 items-center gap-3">
              <span class="package-plan-mark">
                <Icon name="cube" size="md" />
              </span>
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <h2 class="break-words text-lg font-semibold text-gray-900 dark:text-white">{{ selectedName }}</h2>
                  <span class="rounded-md bg-gray-100 px-2 py-1 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-gray-300">
                    {{ selectedCycleLabel }}
                  </span>
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('keyPackages.startsOnSelection') }}
                </p>
              </div>
            </div>
          </div>

          <dl class="package-stats-band">
            <div>
              <dt>{{ t('keyPackages.amount') }}</dt>
              <dd>{{ formatCurrency(selectedAmount) }}</dd>
            </div>
            <div>
              <dt>{{ t('keyPackages.cycle') }}</dt>
              <dd>{{ selectedDays }} {{ t('keyPackages.dayUnit') }}</dd>
            </div>
            <div>
              <dt>5h</dt>
              <dd>{{ formatLimit(calculateLimit(selectedAmount, selectedCycle, 5)) }}</dd>
            </div>
            <div>
              <dt>7d</dt>
              <dd>{{ formatLimit(calculateLimit(selectedAmount, selectedCycle, 168)) }}</dd>
            </div>
          </dl>

          <div class="keys-section-header">
            <div>
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keyPackages.keysTitle') }}</h3>
              <p class="mt-1 text-xs tabular-nums text-gray-500 dark:text-gray-400">
                {{ t('keyPackages.keyCount', { count: keysPagination.total }) }}
              </p>
            </div>
          </div>

          <div
            v-if="keysError"
            class="mx-4 mt-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200 sm:mx-5"
            role="alert"
          >
            <span>{{ keysError }}</span>
            <button v-if="selectedPackage" type="button" class="btn btn-secondary btn-sm" @click="loadKeys(selectedPackage.id)">
              {{ t('keyPackages.retry') }}
            </button>
          </div>

          <div v-if="keysLoading" class="divide-y divide-gray-100 px-4 dark:divide-dark-700 sm:px-5" aria-busy="true">
            <div v-for="item in 3" :key="item" class="py-4">
              <div class="flex items-center justify-between gap-4">
                <div class="min-w-0 flex-1">
                  <Skeleton width="36%" height="17px" />
                  <Skeleton class="mt-2" width="58%" height="14px" />
                </div>
                <Skeleton width="108px" height="40px" />
              </div>
            </div>
          </div>

          <EmptyState
            v-else-if="packageKeys.length === 0 && !keysError"
            class="px-5 py-12"
            :title="t('keyPackages.emptyKeysTitle')"
            :description="t('keyPackages.emptyKeysDescription')"
          >
            <template #icon>
              <Icon name="key" size="xl" class="text-gray-400 dark:text-gray-500" />
            </template>
          </EmptyState>

          <div v-else-if="!keysError" class="divide-y divide-gray-100 dark:divide-dark-700">
            <article v-for="key in packageKeys" :key="key.id" class="package-key-row">
              <div class="key-identity min-w-0">
                <div class="flex min-w-0 flex-wrap items-center gap-2">
                  <h4 class="break-words text-sm font-semibold text-gray-900 dark:text-white">{{ key.name }}</h4>
                  <span :class="statusClass(key.status)">{{ t(`keyPackages.status.${key.status}`) }}</span>
                </div>
                <div class="mt-2 flex min-w-0 items-center gap-2">
                  <code class="min-w-0 break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{ maskApiKey(key.key) }}</code>
                  <button
                    type="button"
                    class="small-icon-action"
                    :aria-label="t('keyPackages.copyKey')"
                    :title="t('keyPackages.copyKey')"
                    @click="copyKey(key.key)"
                  >
                    <Icon name="copy" size="sm" />
                  </button>
                </div>
                <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
                  <span>{{ key.group?.name || t('keys.noGroup') }}</span>
                </div>
                <dl class="package-time-list" data-test="package-time">
                  <div>
                    <dt>{{ t('keyPackages.startsAt') }}</dt>
                    <dd>{{ packageStartText(key) }}</dd>
                  </div>
                  <div>
                    <dt>{{ t('keyPackages.expiresAt') }}</dt>
                    <dd>{{ key.expires_at ? formatDateTime(key.expires_at) : t('keyPackages.noExpiry') }}</dd>
                  </div>
                  <div>
                    <dt>{{ t('keyPackages.remaining') }}</dt>
                    <dd :class="isPackageExpired(key) ? 'text-red-600 dark:text-red-300' : 'text-emerald-700 dark:text-emerald-300'">
                      {{ packageRemainingText(key) }}
                    </dd>
                  </div>
                </dl>
              </div>

              <div class="usage-stack min-w-0" data-test="usage-windows">
                <div v-for="window in keyUsageWindows(key)" :key="window.label" class="usage-window">
                  <div class="flex min-w-0 items-center justify-between gap-3 text-xs">
                    <span class="usage-window-label">{{ window.label }}</span>
                    <span class="min-w-0 truncate font-medium tabular-nums text-gray-800 dark:text-gray-200">
                      {{ t('keyPackages.windowRemaining', { remaining: formatLimit(remainingAmount(window.used, window.limit)), total: formatLimit(window.limit) }) }}
                    </span>
                  </div>
                  <div
                    class="usage-track mt-1.5"
                    role="progressbar"
                    :aria-label="t('keyPackages.windowRemainingLabel', { window: window.label })"
                    :aria-valuetext="t('keyPackages.windowRemaining', { remaining: formatLimit(remainingAmount(window.used, window.limit)), total: formatLimit(window.limit) })"
                    :aria-valuemin="0"
                    :aria-valuemax="window.limit || 1"
                    :aria-valuenow="remainingAmount(window.used, window.limit)"
                  >
                    <div
                      class="usage-track-fill"
                      :class="usageBarClass(window)"
                      :style="{ transform: `scaleX(${remainingPercent(window.used, window.limit) / 100})` }"
                    ></div>
                  </div>
                  <div class="mt-1.5 flex items-center justify-between gap-2 text-xs tabular-nums text-gray-500 dark:text-gray-400">
                    <span class="min-w-0 truncate">{{ windowResetText(window.resetAt) }}</span>
                    <span class="shrink-0 font-medium text-gray-700 dark:text-gray-300">{{ Math.round(remainingPercent(window.used, window.limit)) }}%</span>
                  </div>
                </div>
              </div>

              <div class="package-actions">
                <button
                  v-if="selectedPlan && upgradePlans.length > 0"
                  type="button"
                  class="btn btn-secondary btn-sm min-h-11"
                  :disabled="keyActionId === key.id || isPackageExpired(key)"
                  :aria-label="t('keyPackages.upgradePackage')"
                  :title="isPackageExpired(key) ? t('keyPackages.expiredUpgradeHint') : t('keyPackages.upgradePackage')"
                  :data-test="`upgrade-package-${key.id}`"
                  @click="openUpgradeDialog(key)"
                >
                  <Icon name="trendingUp" size="sm" />
                  {{ t('keyPackages.upgradePackage') }}
                </button>
                <button
                  type="button"
                  class="icon-action"
                  :disabled="keyActionId === key.id"
                  :aria-label="key.status === 'active' ? t('keyPackages.deactivateKey') : t('keyPackages.activateKey')"
                  :title="key.status === 'active' ? t('keyPackages.deactivateKey') : t('keyPackages.activateKey')"
                  @click="toggleKeyStatus(key)"
                >
                  <Icon :name="key.status === 'active' ? 'ban' : 'play'" size="md" />
                </button>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm text-red-600 dark:text-red-300"
                  :disabled="keyActionId === key.id"
                  :aria-label="t('keyPackages.cancelPackage')"
                  :data-test="`cancel-package-${key.id}`"
                  @click="confirmKeyUnbind(key)"
                >
                  <Icon name="xCircle" size="sm" />
                  {{ t('keyPackages.cancelPackage') }}
                </button>
              </div>
            </article>
          </div>

          <div v-if="!keysLoading && !keysError && keysPagination.total > keysPagination.page_size" class="border-t border-gray-100 px-4 py-4 dark:border-dark-700 sm:px-5">
            <Pagination
              :page="keysPagination.page"
              :page-size="keysPagination.page_size"
              :total="keysPagination.total"
              :show-page-size-selector="false"
              @update:page="changeKeysPage"
            />
          </div>
      </section>
    </div>

    <BaseDialog
      :show="keyDialogOpen"
      :title="selectedPlan ? t('keyPackages.bindKeyTitle', { plan: selectedPlan.name }) : t('keyPackages.setKey')"
      width="narrow"
      :close-on-escape="!keySubmitting"
      @close="closeKeyDialog"
    >
      <form id="package-key-form" class="space-y-5" novalidate @submit.prevent="bindPackageKey">
        <div v-if="keyFormError" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200" role="alert">
          {{ keyFormError }}
        </div>

        <dl v-if="selectedPlan" class="dialog-plan-summary">
          <div>
            <dt>{{ t('keyPackages.amount') }}</dt>
            <dd>{{ formatCurrency(selectedPlan.amount_usd) }}</dd>
          </div>
          <div>
            <dt>5h</dt>
            <dd>{{ formatLimit(calculateLimit(selectedPlan.amount_usd, 'month', 5)) }}</dd>
          </div>
          <div>
            <dt>7d</dt>
            <dd>{{ formatLimit(calculateLimit(selectedPlan.amount_usd, 'month', 168)) }}</dd>
          </div>
        </dl>

        <div>
          <label id="package-key-select-label" class="input-label">{{ t('keyPackages.existingKey') }}</label>
          <Select
            v-model="keyForm.key_id"
            :options="availableKeyOptions"
            :placeholder="availableKeysLoading ? t('common.loading') : t('keyPackages.selectExistingKey')"
            :empty-text="t('keyPackages.noAvailableKeys')"
            :loading="availableKeysLoading"
            :disabled="availableKeysLoading"
            searchable
            :aria-label="t('keyPackages.existingKey')"
            aria-describedby="package-key-select-label"
            data-test="key-select"
          />
        </div>
        <div class="dialog-notice">
          <Icon name="infoCircle" size="sm" />
          <p>{{ t('keyPackages.bindKeyNotice') }}</p>
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="keySubmitting" @click="closeKeyDialog">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" form="package-key-form" class="btn btn-primary" data-test="save-key" :disabled="keySubmitting || availableKeysLoading || availableKeys.length === 0">
          <Icon v-if="keySubmitting" name="refresh" size="sm" class="animate-spin" />
          {{ keySubmitting ? t('common.submitting') : t('keyPackages.confirmPlan') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="upgradeDialogOpen"
      :title="pendingKeyUpgrade ? t('keyPackages.upgradePackageTitle', { name: pendingKeyUpgrade.name }) : t('keyPackages.upgradePackage')"
      width="narrow"
      :close-on-escape="!upgradeSubmitting"
      @close="closeUpgradeDialog"
    >
      <form id="package-upgrade-form" class="space-y-5" novalidate @submit.prevent="upgradePackageKey">
        <div v-if="upgradeFormError" class="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200" role="alert">
          {{ upgradeFormError }}
        </div>

        <dl class="upgrade-comparison">
          <div class="min-w-0">
            <dt>{{ t('keyPackages.currentPlan') }}</dt>
            <dd>{{ selectedPlan?.name }}</dd>
            <span>{{ formatCurrency(selectedPlan?.amount_usd ?? 0) }}</span>
          </div>
          <Icon name="arrowRight" size="sm" class="shrink-0 text-gray-400 dark:text-gray-500" />
          <div class="min-w-0 text-right">
            <dt>{{ t('keyPackages.upgradedPlan') }}</dt>
            <dd>{{ selectedUpgradePlan?.name || '-' }}</dd>
            <span>{{ selectedUpgradePlan ? formatCurrency(selectedUpgradePlan.amount_usd) : '-' }}</span>
          </div>
        </dl>

        <div>
          <label id="package-upgrade-select-label" class="input-label">{{ t('keyPackages.targetPlan') }}</label>
          <Select
            v-model="upgradeForm.plan_id"
            :options="upgradePlanOptions"
            :placeholder="t('keyPackages.selectUpgradePlan')"
            :aria-label="t('keyPackages.targetPlan')"
            aria-describedby="package-upgrade-select-label"
            data-test="upgrade-plan-select"
          />
        </div>

        <div class="dialog-notice">
          <Icon name="infoCircle" size="sm" />
          <p>{{ t('keyPackages.upgradeNotice') }}</p>
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary min-h-11" :disabled="upgradeSubmitting" @click="closeUpgradeDialog">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" form="package-upgrade-form" class="btn btn-primary min-h-11" data-test="confirm-upgrade" :disabled="upgradeSubmitting || !upgradeForm.plan_id">
          <Icon v-if="upgradeSubmitting" name="refresh" size="sm" class="animate-spin" />
          {{ upgradeSubmitting ? t('common.submitting') : t('keyPackages.confirmUpgrade') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="keyUnbindOpen"
      :title="t('keyPackages.cancelPackageTitle')"
      :message="pendingKeyUnbind ? t('keyPackages.cancelPackageMessage', { name: pendingKeyUnbind.name }) : ''"
      :confirm-text="t('keyPackages.cancelPackage')"
      danger
      @confirm="unbindKey"
      @cancel="keyUnbindOpen = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { keyPackagesAPI, keysAPI } from '@/api'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Skeleton from '@/components/common/Skeleton.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { maskApiKey } from '@/utils/maskApiKey'
import type {
  ApiKey,
  ApiKeyPackage,
  ApiKeyPackageCycle,
  ApiKeyPackagePlan,
  ApiKeyPackagePlanId,
} from '@/types'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const plans = ref<ApiKeyPackagePlan[]>([])
const packages = ref<ApiKeyPackage[]>([])
const selectedPlanId = ref<ApiKeyPackagePlanId | null>(null)
const packagesLoading = ref(false)
const packagesError = ref('')
const packageKeys = ref<ApiKey[]>([])
const keysLoading = ref(false)
const keysError = ref('')
const availableKeys = ref<ApiKey[]>([])
const availableKeysLoading = ref(false)
const keyActionId = ref<number | null>(null)
const now = ref(new Date())
let keysRequestVersion = 0
let refreshTimer: ReturnType<typeof setInterval> | null = null

const keysPagination = reactive({ page: 1, page_size: 10, total: 0, pages: 0 })
const activePackage = computed(() => packages.value.find((pkg) => pkg.plan_id && pkg.key_count > 0) || null)
const selectedPlan = computed(() => plans.value.find((plan) => plan.id === selectedPlanId.value) || null)
const selectedPackage = computed(() => activePackage.value)
const totalBoundKeys = computed(() => activePackage.value?.key_count ?? 0)
const selectedName = computed(() => selectedPlan.value?.name || selectedPackage.value?.name || '')
const selectedAmount = computed(() => selectedPlan.value?.amount_usd ?? selectedPackage.value?.amount_usd ?? 0)
const selectedCycle = computed<ApiKeyPackageCycle>(() => selectedPlan.value?.cycle ?? selectedPackage.value?.cycle ?? 'month')
const selectedDays = computed(() => selectedCycle.value === 'week' ? 7 : 30)
const selectedCycleLabel = computed(() => selectedCycle.value === 'week' ? t('keyPackages.week') : t('keyPackages.month'))

const keyDialogOpen = ref(false)
const keySubmitting = ref(false)
const keyFormError = ref('')
const keyForm = reactive({ key_id: null as number | null })

const keyUnbindOpen = ref(false)
const pendingKeyUnbind = ref<ApiKey | null>(null)

const upgradeDialogOpen = ref(false)
const upgradeSubmitting = ref(false)
const upgradeFormError = ref('')
const pendingKeyUpgrade = ref<ApiKey | null>(null)
const upgradeForm = reactive({ plan_id: null as ApiKeyPackagePlanId | null })

const availableKeyOptions = computed(() => availableKeys.value.map((key) => ({
  value: key.id,
  label: `${key.name} · ${maskApiKey(key.key)}`,
  description: key.group?.name || t('keys.noGroup'),
})))
const upgradePlans = computed(() => plans.value.filter((plan) => plan.amount_usd > selectedAmount.value))
const upgradePlanOptions = computed(() => upgradePlans.value.map((plan) => ({
  value: plan.id,
  label: plan.name,
  description: `${formatCurrency(plan.amount_usd)} · 5h ${formatLimit(calculateLimit(plan.amount_usd, 'month', 5))} · 7d ${formatLimit(calculateLimit(plan.amount_usd, 'month', 168))}`,
})))
const selectedUpgradePlan = computed(() => upgradePlans.value.find((plan) => plan.id === upgradeForm.plan_id) || null)

function calculateLimit(amount: number, cycle: ApiKeyPackageCycle, hours: number): number {
  if (amount <= 0) return 0
  const cycleHours = cycle === 'week' ? 168 : 720
  return Math.max(0.01, Math.round((amount * hours / cycleHours + Number.EPSILON) * 100) / 100)
}

function formatLimit(amount: number): string {
  return `$${amount.toFixed(2)}`
}

function statusClass(status: ApiKey['status']): string {
  const base = 'rounded-md px-2 py-1 text-xs font-medium'
  if (status === 'active') return `${base} bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300`
  if (status === 'inactive') return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
  if (status === 'quota_exhausted') return `${base} bg-amber-50 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200`
  return `${base} bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300`
}

type UsageWindow = {
  label: '5h' | '7d'
  used: number
  limit: number
  resetAt: string | null
}

function keyUsageWindows(key: ApiKey): UsageWindow[] {
  return [
    { label: '5h', used: key.usage_5h, limit: key.rate_limit_5h, resetAt: key.reset_5h_at },
    { label: '7d', used: key.usage_7d, limit: key.rate_limit_7d, resetAt: key.reset_7d_at },
  ]
}

function remainingAmount(used: number, limit: number): number {
  return Math.max(0, Math.round((Math.max(limit, 0) - Math.max(used, 0) + Number.EPSILON) * 100) / 100)
}

function remainingPercent(used: number, limit: number): number {
  if (limit <= 0) return 0
  return Math.min((remainingAmount(used, limit) / limit) * 100, 100)
}

function usageBarClass(window: UsageWindow): string {
  const percent = remainingPercent(window.used, window.limit)
  if (percent <= 0) return 'bg-red-500'
  if (percent <= 20) return 'bg-amber-500'
  return window.label === '5h' ? 'bg-primary-500' : 'bg-emerald-500'
}

function windowResetText(resetAt: string | null): string {
  if (!resetAt) return t('keyPackages.windowStartsOnUse')
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (diff <= 0) return t('keyPackages.resetSoon')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const minutes = Math.floor((diff % 3600000) / 60000)
  const time = days > 0 ? `${days}d ${hours}h` : hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`
  return t('keyPackages.resetIn', { time })
}

function packageStartText(key: ApiKey): string {
  if (!key.expires_at) return t('keyPackages.noExpiry')
  const start = new Date(key.expires_at)
  if (Number.isNaN(start.getTime())) return t('keyPackages.noExpiry')
  start.setUTCDate(start.getUTCDate() - selectedDays.value)
  return formatDateTime(start.toISOString())
}

function isPackageExpired(key: ApiKey): boolean {
  return !!key.expires_at && new Date(key.expires_at).getTime() <= now.value.getTime()
}

function packageRemainingText(key: ApiKey): string {
  if (!key.expires_at) return t('keyPackages.noExpiry')
  const diff = new Date(key.expires_at).getTime() - now.value.getTime()
  if (!Number.isFinite(diff) || diff <= 0) return t('keyPackages.packageExpired')
  const totalMinutes = Math.max(1, Math.ceil(diff / 60000))
  const days = Math.floor(totalMinutes / 1440)
  const hours = Math.floor((totalMinutes % 1440) / 60)
  const minutes = totalMinutes % 60
  if (days > 0) return t('keyPackages.remainingDaysHours', { days, hours })
  if (hours > 0) return t('keyPackages.remainingHoursMinutes', { hours, minutes })
  return t('keyPackages.remainingMinutes', { minutes })
}

function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object' && 'message' in error && typeof error.message === 'string') {
    return error.message
  }
  return fallback
}

function clearKeys(): void {
  packageKeys.value = []
  keysError.value = ''
  Object.assign(keysPagination, { page: 1, total: 0, pages: 0 })
}

async function loadPage(): Promise<void> {
  packagesLoading.value = true
  packagesError.value = ''
  try {
    const [planResult, packageResult] = await Promise.all([
      keyPackagesAPI.listPlans(),
      keyPackagesAPI.list(),
    ])
    plans.value = planResult
    packages.value = packageResult

    const activePlanID = activePackage.value?.plan_id || null
    selectedPlanId.value = activePlanID && planResult.some((plan) => plan.id === activePlanID)
      ? activePlanID
      : null

    if (selectedPackage.value) {
      await loadKeys(selectedPackage.value.id)
    } else {
      clearKeys()
    }
  } catch (error) {
    packagesError.value = errorMessage(error, t('keyPackages.loadFailed'))
  } finally {
    packagesLoading.value = false
  }
}

async function loadKeys(packageID: number, silent = false): Promise<void> {
  const requestVersion = ++keysRequestVersion
  if (!silent) {
    keysLoading.value = true
    keysError.value = ''
  }
  try {
    const result = await keyPackagesAPI.listKeys(packageID, keysPagination.page, keysPagination.page_size)
    if (requestVersion !== keysRequestVersion || selectedPackage.value?.id !== packageID) return
    packageKeys.value = result.items
    Object.assign(keysPagination, {
      page: result.page,
      page_size: result.page_size,
      total: result.total,
      pages: result.pages,
    })
  } catch (error) {
    if (!silent && requestVersion === keysRequestVersion) {
      clearKeys()
      keysError.value = errorMessage(error, t('keyPackages.keysLoadFailed'))
    }
  } finally {
    if (!silent && requestVersion === keysRequestVersion) keysLoading.value = false
  }
}

function choosePlan(planID: ApiKeyPackagePlanId): void {
  if (activePackage.value) return
  selectedPlanId.value = planID
  clearKeys()
  openBindKey()
}

function changeKeysPage(page: number): void {
  if (!selectedPackage.value || page === keysPagination.page) return
  keysPagination.page = page
  void loadKeys(selectedPackage.value.id)
}

function openBindKey(): void {
  if (!selectedPlan.value || activePackage.value) return
  keyForm.key_id = null
  keyFormError.value = ''
  keyDialogOpen.value = true
  void loadAvailableKeys()
}

function closeKeyDialog(): void {
  if (keySubmitting.value) return
  keyDialogOpen.value = false
  if (!activePackage.value) selectedPlanId.value = null
}

async function loadAvailableKeys(): Promise<void> {
  availableKeysLoading.value = true
  availableKeys.value = []
  try {
    const first = await keyPackagesAPI.listAvailableKeys(1, 100)
    const items = [...first.items]
    for (let page = 2; page <= first.pages; page += 1) {
      const result = await keyPackagesAPI.listAvailableKeys(page, 100)
      items.push(...result.items)
    }
    availableKeys.value = items
  } catch (error) {
    keyFormError.value = errorMessage(error, t('keyPackages.availableKeysLoadFailed'))
  } finally {
    availableKeysLoading.value = false
  }
}

async function bindPackageKey(): Promise<void> {
  if (!selectedPlan.value) return
  if (keyForm.key_id === null) {
    keyFormError.value = t('keyPackages.keyRequired')
    return
  }
  const planID = selectedPlan.value.id
  keySubmitting.value = true
  keyFormError.value = ''
  try {
    const pkg = await keyPackagesAPI.create({ plan_id: planID })
    await keyPackagesAPI.bindKey(pkg.id, { key_id: keyForm.key_id })
    appStore.showSuccess(t('keyPackages.keyBound'))
    keyDialogOpen.value = false
    keysPagination.page = 1
    await loadPage()
  } catch (error) {
    keyFormError.value = errorMessage(error, t('keyPackages.keyBindFailed'))
  } finally {
    keySubmitting.value = false
  }
}

async function copyKey(value: string): Promise<void> {
  await copyToClipboard(value)
}

async function toggleKeyStatus(key: ApiKey): Promise<void> {
  keyActionId.value = key.id
  try {
    const status = key.status === 'active' ? 'inactive' : 'active'
    const updated = await keysAPI.toggleStatus(key.id, status)
    const index = packageKeys.value.findIndex((item) => item.id === key.id)
    if (index >= 0) packageKeys.value[index] = { ...key, ...updated, group: updated.group || key.group }
    appStore.showSuccess(t('keyPackages.keyStatusUpdated'))
  } catch (error) {
    appStore.showError(errorMessage(error, t('keyPackages.keyStatusFailed')))
  } finally {
    keyActionId.value = null
  }
}

function openUpgradeDialog(key: ApiKey): void {
  if (!selectedPlan.value || upgradePlans.value.length === 0 || isPackageExpired(key)) return
  pendingKeyUpgrade.value = key
  upgradeForm.plan_id = null
  upgradeFormError.value = ''
  upgradeDialogOpen.value = true
}

function closeUpgradeDialog(): void {
  if (upgradeSubmitting.value) return
  upgradeDialogOpen.value = false
  pendingKeyUpgrade.value = null
}

async function upgradePackageKey(): Promise<void> {
  const key = pendingKeyUpgrade.value
  const packageID = key?.package_id
  const planID = upgradeForm.plan_id
  if (!key || packageID == null) return
  if (!planID) {
    upgradeFormError.value = t('keyPackages.upgradePlanRequired')
    return
  }
  upgradeSubmitting.value = true
  keyActionId.value = key.id
  upgradeFormError.value = ''
  try {
    await keyPackagesAPI.upgradeKey(packageID, key.id, { plan_id: planID })
    appStore.showSuccess(t('keyPackages.packageUpgraded'))
    upgradeDialogOpen.value = false
    pendingKeyUpgrade.value = null
    keysPagination.page = 1
    await loadPage()
  } catch (error) {
    upgradeFormError.value = errorMessage(error, t('keyPackages.packageUpgradeFailed'))
  } finally {
    upgradeSubmitting.value = false
    keyActionId.value = null
  }
}

function confirmKeyUnbind(key: ApiKey): void {
  pendingKeyUnbind.value = key
  keyUnbindOpen.value = true
}

async function unbindKey(): Promise<void> {
  if (!pendingKeyUnbind.value || !selectedPackage.value) return
  const packageID = selectedPackage.value.id
  const keyID = pendingKeyUnbind.value.id
  keyActionId.value = keyID
  keyUnbindOpen.value = false
  try {
    await keyPackagesAPI.unbindKey(packageID, keyID)
    appStore.showSuccess(t('keyPackages.packageCancelled'))
    if (packageKeys.value.length === 1 && keysPagination.page > 1) keysPagination.page -= 1
    pendingKeyUnbind.value = null
    await loadPage()
  } catch (error) {
    appStore.showError(errorMessage(error, t('keyPackages.keyUnbindFailed')))
  } finally {
    keyActionId.value = null
  }
}

onMounted(() => {
  void loadPage()
  refreshTimer = setInterval(() => {
    now.value = new Date()
    if (selectedPackage.value && !keysLoading.value) void loadKeys(selectedPackage.value.id, true)
  }, 60000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<style scoped>
.package-workspace {
  @apply min-w-0 space-y-5;
}

.package-visual-header {
  @apply relative isolate min-h-[164px] min-w-0 overflow-hidden rounded-lg border border-slate-700/70 bg-slate-950 shadow-card;
}

.package-visual-image {
  @apply absolute inset-0 h-full w-full object-cover;
  object-position: 58% 52%;
}

.package-visual-scrim {
  @apply absolute inset-0;
  background: linear-gradient(90deg, rgb(4 9 13 / 0.97) 0%, rgb(4 9 13 / 0.9) 40%, rgb(4 9 13 / 0.38) 72%, rgb(4 9 13 / 0.12) 100%);
}

.package-visual-content {
  @apply relative z-[1] flex min-h-[164px] min-w-0 flex-col justify-between gap-5 p-5 sm:flex-row sm:items-end sm:p-6;
}

.package-visual-copy {
  @apply min-w-0;
}

.package-visual-copy h1 {
  @apply text-2xl font-semibold text-slate-50 sm:text-[1.75rem];
}

.package-visual-copy p {
  @apply mt-2 max-w-xl text-sm leading-6 text-slate-300;
}

.package-visual-actions {
  @apply flex min-w-0 flex-wrap items-center gap-2 sm:justify-end;
}

.visual-chip {
  @apply inline-flex min-h-11 min-w-0 items-center gap-2 rounded-md border border-white/15 bg-slate-950/70 px-3 text-xs font-medium tabular-nums text-slate-100 shadow-sm backdrop-blur-md;
}

.visual-chip :deep(svg) {
  @apply shrink-0 text-cyan-300;
}

.visual-icon-action {
  @apply inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-md border border-white/15 bg-slate-950/70 text-slate-200 shadow-sm backdrop-blur-md transition-colors duration-150;
  @apply hover:border-white/30 hover:bg-slate-900 focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-300/70 disabled:cursor-not-allowed disabled:opacity-40;
}

.plan-section {
  @apply min-w-0;
}

.plan-section-header {
  @apply mb-4 flex min-w-0 items-end justify-between gap-3 px-0.5;
}

.plan-grid {
  display: grid;
  grid-auto-columns: clamp(248px, calc(100vw - 3.5rem), 312px);
  grid-auto-flow: column;
  gap: 0.75rem;
  min-width: 0;
  overflow-x: auto;
  overscroll-behavior-x: contain;
  padding: 0.125rem 0.125rem 0.875rem;
  scroll-padding-inline: 0.125rem;
  scroll-snap-type: x proximity;
  scrollbar-color: rgb(100 116 139 / 0.55) transparent;
  scrollbar-width: thin;
}

.plan-card {
  --plan-accent: rgb(15 118 110);
  @apply relative flex min-h-[378px] min-w-0 flex-col overflow-hidden rounded-lg border border-slate-200 bg-white p-5 text-slate-900 transition-[border-color,box-shadow,transform] duration-200;
  box-shadow: 0 1px 2px rgb(15 23 42 / 0.04), 0 16px 36px -28px rgb(15 23 42 / 0.42);
  scroll-snap-align: start;
}

.plan-card[data-plan='go'] {
  --plan-accent: rgb(5 150 105);
}

.plan-card[data-plan='plus'] {
  --plan-accent: rgb(8 145 178);
}

.plan-card[data-plan='pro_5x'] {
  --plan-accent: rgb(79 70 229);
}

.plan-card[data-plan='pro_20x'] {
  --plan-accent: rgb(225 29 72);
}

.plan-card[data-plan='pro_30x'] {
  --plan-accent: rgb(217 119 6);
}

.plan-card:hover {
  border-color: var(--plan-accent);
  box-shadow: 0 4px 8px rgb(15 23 42 / 0.07), 0 22px 48px -30px rgb(15 23 42 / 0.55);
  transform: translateY(-2px);
}

.plan-card-selected {
  z-index: 1;
  border-color: var(--plan-accent);
  box-shadow: 0 0 0 1px var(--plan-accent), 0 22px 48px -30px rgb(15 23 42 / 0.62);
}

.plan-accent {
  @apply absolute inset-x-0 top-0 h-1;
  background: var(--plan-accent);
}

.plan-card-meta {
  @apply flex min-h-7 min-w-0 flex-wrap items-center gap-2;
}

.plan-sequence {
  @apply inline-flex h-7 min-w-7 items-center justify-center rounded-md bg-slate-100 px-1.5 font-mono text-[11px] font-semibold tabular-nums text-slate-500;
}

.plan-cycle-label {
  @apply inline-flex min-h-7 min-w-0 items-center gap-1.5 text-xs font-medium text-slate-500;
}

.plan-cycle-label :deep(svg) {
  @apply shrink-0;
}

.recommended-badge {
  @apply absolute right-4 top-4 shrink-0 rounded-md border border-cyan-200 bg-cyan-50 px-2 py-1 text-[11px] font-semibold text-cyan-800;
}

.plan-copy {
  @apply mt-6 min-w-0;
}

.plan-name {
  @apply break-words text-[1.35rem] font-semibold text-slate-950;
}

.plan-description {
  @apply mt-2 min-h-10 text-sm leading-5 text-slate-500;
}

.plan-price-row {
  @apply mt-6 flex min-w-0 items-end gap-1.5;
}

.plan-price {
  @apply min-w-0 whitespace-nowrap text-[1.75rem] font-semibold tabular-nums text-slate-950;
}

.plan-price-compact {
  @apply text-[1.4rem];
}

.plan-price-unit {
  @apply whitespace-nowrap pb-1 text-xs text-slate-500;
}

.plan-limits {
  @apply mt-5 grid grid-cols-2 divide-x divide-slate-200 border-y border-slate-200;
}

.plan-limits > div {
  @apply min-w-0 px-3 py-2.5 first:pl-0 last:pr-0;
}

.plan-limits dt {
  @apply flex items-center gap-1.5 text-xs font-medium text-slate-500;
}

.plan-limits dd {
  @apply mt-0.5 truncate text-sm font-semibold tabular-nums text-slate-900;
}

.limit-dot {
  @apply h-1.5 w-1.5 shrink-0 rounded-full;
  background: var(--plan-accent);
}

.plan-select-button {
  @apply mt-auto inline-flex min-h-11 w-full items-center justify-between gap-3 rounded-md border border-slate-900 bg-slate-950 px-3.5 text-sm font-semibold text-white transition-[background-color,border-color,color,transform] duration-150;
  @apply hover:border-slate-700 hover:bg-slate-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 active:scale-[0.98];
}

.plan-card-featured:not(.plan-card-selected) .plan-select-button {
  @apply border-cyan-700 bg-cyan-700 text-white hover:border-cyan-600 hover:bg-cyan-600;
}

.plan-select-button-selected {
  border-color: var(--plan-accent);
}

.package-detail-panel {
  @apply min-w-0 overflow-hidden rounded-lg border border-slate-200/80 bg-white shadow-card-hover dark:border-dark-700 dark:bg-dark-900;
  min-height: 27rem;
}

.package-detail-header,
.keys-section-header {
  @apply flex min-h-16 flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:px-5;
}

.package-plan-mark {
  @apply flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-slate-950 text-cyan-300 ring-1 ring-slate-800 dark:bg-dark-950 dark:text-cyan-300 dark:ring-dark-700;
}

.keys-section-header {
  @apply bg-white dark:bg-dark-900;
}

.package-stats-band {
  @apply grid grid-cols-2 divide-x divide-y divide-gray-100 border-b border-gray-100 bg-gray-50/70 dark:divide-dark-700 dark:border-dark-700 dark:bg-dark-950/40 sm:grid-cols-4 sm:divide-y-0;
}

.package-stats-band > div {
  @apply min-w-0 px-4 py-3 sm:px-5;
}

.package-stats-band dt,
.dialog-plan-summary dt {
  @apply text-xs text-gray-500 dark:text-gray-400;
}

.package-stats-band dd,
.dialog-plan-summary dd {
  @apply mt-1 break-words text-sm font-semibold tabular-nums text-gray-900 dark:text-white;
}

.package-key-row {
  @apply grid min-w-0 gap-5 px-4 py-5 sm:px-5 lg:grid-cols-[minmax(300px,1.15fr)_minmax(260px,1fr)_auto] lg:items-center;
}

.package-time-list {
  @apply mt-4 grid border-y border-gray-100 bg-gray-50/60 py-1 dark:border-dark-700 dark:bg-dark-950/35;
}

.package-time-list > div {
  @apply grid min-w-0 grid-cols-[5rem_minmax(0,1fr)] items-baseline gap-x-3 px-2 py-1.5;
}

.package-time-list dt {
  @apply text-xs text-gray-500 dark:text-gray-400;
}

.package-time-list dd {
  @apply min-w-0 break-words text-right text-xs font-medium tabular-nums text-gray-800 dark:text-gray-200;
}

.usage-stack {
  @apply grid gap-4 border-t border-gray-100 pt-4 dark:border-dark-700 lg:border-l lg:border-t-0 lg:pl-5 lg:pt-0;
}

.usage-window + .usage-window {
  @apply border-t border-gray-100 pt-4 dark:border-dark-700;
}

.usage-window-label {
  @apply inline-flex min-w-9 items-center justify-center rounded-md bg-slate-100 px-2 py-1 font-semibold text-slate-700 dark:bg-dark-700 dark:text-gray-200;
}

.usage-track {
  @apply h-3 w-full overflow-hidden rounded-full bg-slate-200 ring-1 ring-inset ring-slate-300/70 dark:bg-dark-700 dark:ring-dark-600;
}

.usage-track-fill {
  @apply h-full origin-left rounded-full transition-transform duration-300 motion-reduce:transition-none;
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.35);
}

.package-actions {
  @apply flex min-w-0 flex-wrap items-center gap-2 lg:justify-end;
}

.package-actions .btn {
  @apply min-w-0 flex-1 lg:flex-none;
}

.icon-action,
.small-icon-action {
  @apply inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-500 transition-colors duration-150;
  @apply hover:border-gray-300 hover:bg-gray-50 hover:text-gray-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/50;
  @apply disabled:cursor-not-allowed disabled:opacity-40 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-dark-500 dark:hover:bg-dark-700 dark:hover:text-white;
}

.small-icon-action {
  @apply border-0 bg-transparent dark:bg-transparent;
}

.dialog-plan-summary {
  @apply grid grid-cols-3 divide-x divide-gray-100 border-y border-gray-100 bg-gray-50/60 py-3 dark:divide-dark-700 dark:border-dark-700 dark:bg-dark-950/35;
}

.dialog-plan-summary > div {
  @apply min-w-0 px-3 first:pl-0 last:pr-0;
}

.upgrade-comparison {
  @apply grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-3 border-y border-gray-100 bg-gray-50/60 px-3 py-3 dark:border-dark-700 dark:bg-dark-950/35;
}

.upgrade-comparison dt {
  @apply text-xs text-gray-500 dark:text-gray-400;
}

.upgrade-comparison dd {
  @apply mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white;
}

.upgrade-comparison span {
  @apply mt-0.5 block truncate text-xs font-medium tabular-nums text-gray-500 dark:text-gray-400;
}

.dialog-notice {
  @apply flex items-start gap-2.5 border-l-2 border-primary-500 bg-primary-50/60 px-3 py-2.5 text-xs leading-5 text-primary-900 dark:border-primary-400 dark:bg-primary-950/20 dark:text-primary-100;
}

.dialog-notice :deep(svg) {
  @apply mt-0.5 shrink-0 text-primary-600 dark:text-primary-300;
}

@media (max-width: 1023px) {
  .package-key-row > :last-child {
    @apply justify-start;
  }
}

@media (min-width: 1340px) {
  .plan-grid {
    grid-auto-flow: row;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    overflow: visible;
    padding: 0.125rem;
    scroll-snap-type: none;
  }
}

@media (min-width: 1024px) {
  .package-visual-content {
    @apply px-7;
  }
}

@media (max-width: 639px) {
  .package-visual-header,
  .package-visual-content {
    min-height: 220px;
  }

  .package-visual-image {
    object-position: 62% 52%;
  }

  .package-visual-scrim {
    background: linear-gradient(180deg, rgb(4 9 13 / 0.87) 0%, rgb(4 9 13 / 0.72) 48%, rgb(4 9 13 / 0.94) 100%);
  }

  .package-visual-actions {
    @apply w-full;
  }

  .visual-chip {
    @apply flex-1 justify-center;
  }

  .package-detail-header {
    @apply items-start;
  }

  .package-stats-band > div {
    @apply px-3;
  }
}
</style>

<style>
/* Vue's scoped transform can drop a root-level dark selector; keep theme overrides unscoped. */
.dark .package-workspace .plan-grid {
  scrollbar-color: rgb(148 163 184 / 0.65) transparent;
}

.dark .package-workspace .plan-card {
  border-color: rgb(51 65 85);
  color: rgb(241 245 249);
  background: rgb(12 20 32);
  box-shadow: 0 1px 2px rgb(0 0 0 / 0.28), 0 18px 42px -30px rgb(0 0 0 / 0.85);
}

.dark .package-workspace .plan-card:hover {
  border-color: var(--plan-accent);
  background: rgb(15 25 40);
  box-shadow: 0 4px 10px rgb(0 0 0 / 0.34), 0 24px 52px -32px rgb(0 0 0 / 0.92);
}

.dark .package-workspace .plan-card-selected {
  border-color: var(--plan-accent);
  box-shadow: 0 0 0 1px var(--plan-accent), 0 24px 52px -32px rgb(0 0 0 / 0.92);
}

.dark .package-workspace .plan-cycle-label,
.dark .package-workspace .plan-price-unit,
.dark .package-workspace .plan-limits dt {
  color: rgb(148 163 184);
}

.dark .package-workspace .plan-sequence {
  background: rgb(30 41 59);
  color: rgb(148 163 184);
}

.dark .package-workspace .recommended-badge {
  border-color: rgb(103 232 249 / 0.22);
  background: rgb(103 232 249 / 0.1);
  color: rgb(165 243 252);
}

.dark .package-workspace .plan-name,
.dark .package-workspace .plan-price {
  color: rgb(255 255 255);
}

.dark .package-workspace .plan-description {
  color: rgb(148 163 184);
}

.dark .package-workspace .plan-limits {
  border-color: rgb(255 255 255 / 0.1);
}

.dark .package-workspace .plan-limits > div + div {
  border-color: rgb(255 255 255 / 0.1);
}

.dark .package-workspace .plan-limits dd {
  color: rgb(241 245 249);
}

.dark .package-workspace .plan-select-button {
  border-color: rgb(226 232 240);
  background: rgb(241 245 249);
  color: rgb(2 6 23);
}

.dark .package-workspace .plan-select-button:hover {
  border-color: rgb(255 255 255);
  background: rgb(255 255 255);
}

.dark .package-workspace .plan-card-featured:not(.plan-card-selected) .plan-select-button {
  border-color: rgb(8 145 178);
  background: rgb(8 145 178);
  color: rgb(255 255 255);
}

.dark .package-workspace .plan-select-button-selected {
  border-color: var(--plan-accent);
}

.dark .package-workspace .plan-select-button-selected:hover {
  border-color: rgb(255 255 255);
  background: rgb(255 255 255);
}
</style>
