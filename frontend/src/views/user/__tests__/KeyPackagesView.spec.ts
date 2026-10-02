import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import KeyPackagesView from '../KeyPackagesView.vue'
import type { ApiKey, ApiKeyPackage, ApiKeyPackagePlan } from '@/types'

const {
  listPlans,
  listPackages,
  createPackage,
  listPackageKeys,
  listAvailableKeys,
  bindKey,
  upgradeKey,
  unbindKey,
  toggleKeyStatus,
  deleteKey,
  showSuccess,
  showError,
  copyToClipboard,
} = vi.hoisted(() => ({
  listPlans: vi.fn(),
  listPackages: vi.fn(),
  createPackage: vi.fn(),
  listPackageKeys: vi.fn(),
  listAvailableKeys: vi.fn(),
  bindKey: vi.fn(),
  upgradeKey: vi.fn(),
  unbindKey: vi.fn(),
  toggleKeyStatus: vi.fn(),
  deleteKey: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  copyToClipboard: vi.fn(),
}))

vi.mock('@/api', () => ({
  keyPackagesAPI: {
    listPlans,
    list: listPackages,
    create: createPackage,
    listKeys: listPackageKeys,
    listAvailableKeys,
    bindKey,
    upgradeKey,
    unbindKey,
  },
  keysAPI: {
    toggleStatus: toggleKeyStatus,
    delete: deleteKey,
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard }),
}))

vi.mock('@/utils/format', () => ({
  formatCurrency: (amount: number) => `$${amount.toFixed(2)}`,
  formatDateTime: (value: string) => value,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const messages: Record<string, string> = {
    'common.cancel': 'Cancel',
    'common.loading': 'Loading',
    'common.refresh': 'Refresh',
    'common.submitting': 'Submitting',
    'keyPackages.title': 'Package Management',
    'keyPackages.description': 'Fixed plans',
    'keyPackages.choosePlan': 'Choose a plan',
    'keyPackages.planPeriodNotice': '30 days from binding',
    'keyPackages.totalBoundKeys': '{count} keys assigned',
    'keyPackages.recommended': 'Recommended',
    'keyPackages.selectAndSetKey': 'Select and set key',
    'keyPackages.monthUnit': 'month',
    'keyPackages.dayUnit': 'days',
    'keyPackages.thirtyDays': 'Valid for 30 days',
    'keyPackages.limitValue': '{window} limit {amount}',
    'keyPackages.plans.go': 'Light usage',
    'keyPackages.plans.plus': 'Everyday usage',
    'keyPackages.plans.pro_5x': 'Advanced usage',
    'keyPackages.plans.pro_20x': 'Professional usage',
    'keyPackages.plans.pro_30x': 'Highest usage',
    'keyPackages.retry': 'Retry',
    'keyPackages.amount': 'Amount',
    'keyPackages.cycle': 'Period',
    'keyPackages.week': 'Weekly',
    'keyPackages.month': 'Monthly',
    'keyPackages.keyCount': '{count} keys',
    'keyPackages.keysTitle': 'Plan keys',
    'keyPackages.setKey': 'Set key',
    'keyPackages.startsOnSelection': 'Starts after binding',
    'keyPackages.bindKeyTitle': 'Set a key for {plan}',
    'keyPackages.existingKey': 'Existing API key',
    'keyPackages.selectExistingKey': 'Select an existing key',
    'keyPackages.noAvailableKeys': 'No keys available',
    'keyPackages.bindKeyNotice': 'Starts now for 30 days',
    'keyPackages.confirmPlan': 'Confirm plan',
    'keyPackages.emptyKeysTitle': 'No keys yet',
    'keyPackages.emptyKeysDescription': 'Select a key',
    'keyPackages.status.active': 'Active',
    'keyPackages.expires': 'Expires {time}',
    'keyPackages.noExpiry': 'No expiry',
    'keyPackages.startsAt': 'Starts',
    'keyPackages.expiresAt': 'Expires',
    'keyPackages.remaining': 'Remaining',
    'keyPackages.remainingDaysHours': '{days}d {hours}h',
    'keyPackages.remainingHoursMinutes': '{hours}h {minutes}m',
    'keyPackages.remainingMinutes': '{minutes}m',
    'keyPackages.packageExpired': 'Expired',
    'keyPackages.upgradePackage': 'Upgrade plan',
    'keyPackages.upgradePackageTitle': 'Upgrade {name}',
    'keyPackages.targetPlan': 'Upgrade to',
    'keyPackages.selectUpgradePlan': 'Select a higher plan',
    'keyPackages.currentPlan': 'Current plan',
    'keyPackages.upgradedPlan': 'After upgrade',
    'keyPackages.upgradeNotice': 'Period and usage are not reset',
    'keyPackages.confirmUpgrade': 'Confirm upgrade',
    'keyPackages.expiredUpgradeHint': 'Cancel first',
    'keyPackages.windowRemaining': 'Remaining {remaining} / {total}',
    'keyPackages.windowRemainingLabel': '{window} remaining quota',
    'keyPackages.windowStartsOnUse': 'Starts on first use',
    'keyPackages.resetIn': 'Resets in {time}',
    'keyPackages.resetSoon': 'Resetting soon',
    'keyPackages.copyKey': 'Copy API key',
    'keyPackages.deactivateKey': 'Deactivate key',
    'keyPackages.activateKey': 'Activate key',
    'keyPackages.cancelPackage': 'Cancel plan',
    'keyPackages.cancelPackageTitle': 'Cancel plan',
    'keyPackages.cancelPackageMessage': 'Cancel {name}',
    'keyPackages.packageCancelled': 'Plan cancelled',
    'keys.noGroup': 'No group',
  }
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        let value = messages[key] ?? key
        for (const [name, replacement] of Object.entries(params ?? {})) {
          value = value.replace(`{${name}}`, String(replacement))
        }
        return value
      },
    }),
  }
})

const plans: ApiKeyPackagePlan[] = [
  { id: 'go', name: 'Go', cycle: 'month', amount_usd: 70, limit_5h_enabled: true, limit_7d_enabled: true },
  { id: 'plus', name: 'Plus', cycle: 'month', amount_usd: 140, limit_5h_enabled: true, limit_7d_enabled: true },
  { id: 'pro_5x', name: 'Pro 5x', cycle: 'month', amount_usd: 700, limit_5h_enabled: true, limit_7d_enabled: true },
  { id: 'pro_20x', name: 'Pro 20x', cycle: 'month', amount_usd: 1400, limit_5h_enabled: true, limit_7d_enabled: true },
  { id: 'pro_30x', name: 'Pro 30x', cycle: 'month', amount_usd: 1800, limit_5h_enabled: true, limit_7d_enabled: true },
]

const plusPackage: ApiKeyPackage = {
  id: 2,
  user_id: 7,
  plan_id: 'plus',
  name: 'Plus',
  cycle: 'month',
  amount_usd: 140,
  limit_5h_enabled: true,
  limit_7d_enabled: true,
  key_count: 1,
  created_at: '2026-08-20T00:00:00Z',
  updated_at: '2026-08-21T00:00:00Z',
}

const packageKey: ApiKey = {
  id: 11,
  user_id: 7,
  package_id: 2,
  key: 'sk-package-test-key',
  name: 'Production',
  group_id: 9,
  mixed_mode_enabled: false,
  mixed_group_ids: [],
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 140,
  quota_used: 12.5,
  expires_at: '2026-09-19T00:00:00Z',
  created_at: '2026-08-21T00:00:00Z',
  updated_at: '2026-08-21T00:00:00Z',
  current_concurrency: 0,
  rate_limit_5h: 0.97,
  rate_limit_1d: 0,
  rate_limit_7d: 32.67,
  usage_5h: 0.49,
  usage_1d: 0,
  usage_7d: 16.34,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
}

const availableKey: ApiKey = {
  ...packageKey,
  id: 22,
  package_id: null,
  key: 'sk-existing-unassigned-key',
  name: 'Staging',
  quota: 0,
  quota_used: 0,
  expires_at: null,
  rate_limit_5h: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_7d: 0,
}

const AppLayoutStub = { template: '<div><slot /></div>' }
const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div v-if="show" data-test="base-dialog"><h2>{{ title }}</h2><slot /><slot name="footer" /></div>',
}
const ConfirmDialogStub = {
  props: ['show'],
  emits: ['confirm', 'cancel'],
  template: '<div v-if="show"><button data-test="confirm-dialog-confirm" @click="$emit(\'confirm\')">Confirm</button></div>',
}
const EmptyStateStub = {
  props: ['title', 'description', 'actionText'],
  emits: ['action'],
  template: '<div><slot name="icon" /><span>{{ title }}</span><span>{{ description }}</span><button v-if="actionText" @click="$emit(\'action\')">{{ actionText }}</button></div>',
}
const PaginationStub = { template: '<div />' }
const SkeletonStub = { template: '<div />' }
const IconStub = { props: ['name'], template: '<span />' }
const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<select :value="modelValue ?? \'\'" @change="$emit(\'update:modelValue\', options.find((option) => String(option.value) === $event.target.value)?.value)"><option value="">Select</option><option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>',
}

function mountView() {
  return mount(KeyPackagesView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: ConfirmDialogStub,
        EmptyState: EmptyStateStub,
        Pagination: PaginationStub,
        Select: SelectStub,
        Skeleton: SkeletonStub,
        Icon: IconStub,
      },
    },
  })
}

describe('KeyPackagesView', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-09-01T00:00:00Z'))
    vi.clearAllMocks()
    listPlans.mockResolvedValue(plans)
    listPackages.mockResolvedValue([plusPackage])
    listPackageKeys.mockImplementation(async (packageId: number) => ({
      items: packageId === 2 ? [packageKey] : [],
      total: packageId === 2 ? 1 : 0,
      page: 1,
      page_size: 10,
      pages: packageId === 2 ? 1 : 0,
    }))
    listAvailableKeys.mockResolvedValue({
      items: [availableKey],
      total: 1,
      page: 1,
      page_size: 100,
      pages: 1,
    })
    createPackage.mockResolvedValue({ ...plusPackage, id: 20, plan_id: 'pro_20x', name: 'Pro 20x', amount_usd: 1400, key_count: 0 })
    bindKey.mockResolvedValue({ ...availableKey, package_id: 20 })
    upgradeKey.mockResolvedValue({ ...packageKey, package_id: 20, quota: 700, rate_limit_5h: 4.86, rate_limit_7d: 163.33 })
    unbindKey.mockResolvedValue({ ...packageKey, package_id: null })
    copyToClipboard.mockResolvedValue(true)
  })

  afterEach(() => vi.useRealTimers())

  it('renders only the five fixed monthly plans and their prices', async () => {
    listPackages.mockResolvedValueOnce([])
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[data-test^="plan-card-"]')).toHaveLength(5)
    expect(wrapper.text()).toContain('Go')
    expect(wrapper.text()).toContain('Plus')
    expect(wrapper.text()).toContain('Pro 5x')
    expect(wrapper.text()).toContain('Pro 20x')
    expect(wrapper.text()).toContain('Pro 30x')
    for (const price of ['$70.00', '$140.00', '$700.00', '$1400.00', '$1800.00']) {
      expect(wrapper.text()).toContain(price)
    }
    expect(wrapper.find('.package-detail-panel').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Legacy plans')
  })

  it('shows only the active package details after a plan is assigned', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[data-test^="plan-card-"]')).toHaveLength(0)
    expect(wrapper.get('.package-detail-panel').text()).toContain('Plus')
    expect(wrapper.get('.package-detail-panel').text()).toContain('Production')
    expect(wrapper.findAll('[role="progressbar"]')).toHaveLength(2)
    expect(wrapper.find('[data-test="bind-key"]').exists()).toBe(false)
  })

  it('ignores legacy package data in the package selector', async () => {
    listPackages.mockResolvedValueOnce([{
      ...plusPackage,
      id: 99,
      plan_id: '',
      name: 'Legacy test',
      amount_usd: 99,
      key_count: 1,
    }])
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[data-test^="plan-card-"]')).toHaveLength(5)
    expect(wrapper.text()).not.toContain('Legacy test')
  })

  it('shows two dynamic remaining-quota bars with two-decimal values', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[role="progressbar"]')).toHaveLength(2)
    expect(wrapper.findAll('[role="progressbar"]')[0].attributes('aria-label')).toBe('5h remaining quota')
    expect(wrapper.findAll('[role="progressbar"]')[1].attributes('aria-label')).toBe('7d remaining quota')
    expect(wrapper.text()).toContain('Remaining $0.48 / $0.97')
    expect(wrapper.text()).toContain('Remaining $16.33 / $32.67')
    expect(wrapper.text()).toContain('49%')
    expect(wrapper.text()).toContain('50%')
    expect(wrapper.text().match(/Starts on first use/g)).toHaveLength(2)
  })

  it('shows 100% remaining before any package usage', async () => {
    listPackageKeys.mockResolvedValueOnce({
      items: [{ ...packageKey, usage_5h: 0, usage_7d: 0 }],
      total: 1,
      page: 1,
      page_size: 10,
      pages: 1,
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text().match(/100%/g)).toHaveLength(2)
    expect(wrapper.text()).toContain('Remaining $0.97 / $0.97')
    expect(wrapper.text()).toContain('Remaining $32.67 / $32.67')
  })

  it('shows the plan start, expiry, and remaining time for each key', async () => {
    const wrapper = mountView()
    await flushPromises()

    const time = wrapper.get('[data-test="package-time"]')
    expect(time.text()).toContain('Starts')
    expect(time.text()).toContain('2026-08-20T00:00:00.000Z')
    expect(time.text()).toContain('Expires')
    expect(time.text()).toContain('2026-09-19T00:00:00Z')
    expect(time.text()).toContain('Remaining')
  })

  it('upgrades only to a higher plan without canceling or rebinding the key', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="upgrade-package-11"]').trigger('click')
    const select = wrapper.get('[data-test="upgrade-plan-select"]')
    const options = select.findAll('option').map((option) => option.text())
    expect(options).not.toContain('Go')
    expect(options).not.toContain('Plus')
    expect(options).toContain('Pro 5x')
    expect(options).toContain('Pro 20x')
    expect(options).toContain('Pro 30x')

    await select.setValue('pro_5x')
    await wrapper.get('#package-upgrade-form').trigger('submit')
    await flushPromises()

    expect(upgradeKey).toHaveBeenCalledWith(2, 11, { plan_id: 'pro_5x' })
    expect(unbindKey).not.toHaveBeenCalled()
    expect(bindKey).not.toHaveBeenCalled()
  })

  it('selects a fixed plan and binds an existing unassigned key', async () => {
    listPackages
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([{ ...plusPackage, id: 20, plan_id: 'pro_20x', name: 'Pro 20x', amount_usd: 1400 }])
    listPackageKeys.mockImplementation(async (packageId: number) => ({
      items: packageId === 20 ? [{ ...packageKey, package_id: 20 }] : [],
      total: packageId === 20 ? 1 : 0,
      page: 1,
      page_size: 10,
      pages: packageId === 20 ? 1 : 0,
    }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="select-plan-pro_20x"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="key-select"]').setValue('22')
    await wrapper.get('#package-key-form').trigger('submit')
    await flushPromises()

    expect(listAvailableKeys).toHaveBeenCalledWith(1, 100)
    expect(createPackage).toHaveBeenCalledWith({ plan_id: 'pro_20x' })
    expect(bindKey).toHaveBeenCalledWith(20, { key_id: 22 })
    expect(wrapper.findAll('[data-test^="plan-card-"]')).toHaveLength(0)
    expect(wrapper.get('.package-detail-panel').text()).toContain('Pro 20x')
  })

  it('does not keep a plan selected when key setup is cancelled', async () => {
    listPackages.mockResolvedValueOnce([])
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="select-plan-plus"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="plan-card-plus"]').classes()).toContain('plan-card-selected')

    await wrapper.get('[data-test="base-dialog"]').find('.btn-secondary').trigger('click')
    expect(wrapper.get('[data-test="plan-card-plus"]').classes()).not.toContain('plan-card-selected')
  })

  it('cancels the plan without deleting the underlying key', async () => {
    listPackages.mockResolvedValueOnce([plusPackage]).mockResolvedValueOnce([])
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="cancel-package-11"]').trigger('click')
    await wrapper.get('[data-test="confirm-dialog-confirm"]').trigger('click')
    await flushPromises()

    expect(unbindKey).toHaveBeenCalledWith(2, 11)
    expect(deleteKey).not.toHaveBeenCalled()
    expect(wrapper.findAll('[data-test^="plan-card-"]')).toHaveLength(5)
    expect(wrapper.find('.package-detail-panel').exists()).toBe(false)
  })
})
