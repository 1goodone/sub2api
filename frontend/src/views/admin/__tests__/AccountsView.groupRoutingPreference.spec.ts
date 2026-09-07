import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const { listAccounts, listWithEtag, getAllGroups, updateGroup, showSuccess } = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getAllGroups: vi.fn(),
  updateGroup: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats: vi.fn().mockResolvedValue({ stats: {} }),
      getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({ enabled: true, interval_minutes: 30 })
    },
    proxies: {
      getAll: vi.fn().mockResolvedValue([])
    },
    groups: {
      getAll: getAllGroups,
      update: updateGroup
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess,
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token',
    isSimpleMode: false
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const AccountTableFiltersStub = {
  props: ['filters'],
  emits: ['update:filters', 'change', 'update:searchQuery'],
  template: `
    <button
      data-test="select-routing-group"
      @click="$emit('update:filters', { group: '7' })"
    >
      select group
    </button>
  `
}

const GroupCellDataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-groups" :row="row" />
      </div>
    </div>
  `
}

describe('admin AccountsView selected group routing preference', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset()
    listWithEtag.mockReset()
    getAllGroups.mockReset()
    updateGroup.mockReset()
    showSuccess.mockReset()

    listAccounts.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getAllGroups.mockResolvedValue([
      { id: 7, name: 'OpenAI low rate', platform: 'openai', account_type_preference: 'default' }
    ])
    updateGroup.mockResolvedValue({
      id: 7,
      name: 'OpenAI low rate',
      platform: 'openai',
      account_type_preference: 'apikey_first'
    })
  })

  it('updates the selected group preference outside account rows', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          AccountTableFilters: AccountTableFiltersStub,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountBulkActionsBar: true,
          DataTable: true,
          Pagination: true,
          ConfirmDialog: true,
          AccountActionMenu: true,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: true,
          TotpStepUpDialog: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    expect(wrapper.find('[data-test="selected-group-routing-preference"]').exists()).toBe(false)

    await wrapper.get('[data-test="select-routing-group"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="selected-group-routing-preference"]').text()).toContain('OpenAI low rate')

    await wrapper.get('[data-test="group-routing-preference-apikey_first"]').trigger('click')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledWith(7, { account_type_preference: 'apikey_first' })
    expect(showSuccess).toHaveBeenCalledWith('admin.groups.groupUpdated')
  })

  it('adds an account without replacing existing focus pool members', async () => {
    const group = {
      id: 7,
      name: 'OpenAI low rate',
      platform: 'openai',
      focus_account_id: 41,
      focus_account_ids: [41, 42]
    }
    getAllGroups.mockResolvedValue([group])
    listAccounts.mockResolvedValue({
      items: [{
        id: 43,
        name: 'focus candidate',
        platform: 'openai',
        type: 'oauth',
        status: 'active',
        schedulable: true,
        group_ids: [7],
        groups: [group]
      }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    updateGroup.mockResolvedValue({ ...group, focus_account_ids: [41, 42, 43] })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          AccountTableFilters: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountBulkActionsBar: true,
          DataTable: GroupCellDataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountActionMenu: true,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: true,
          TotpStepUpDialog: true,
          Teleport: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('button[title="admin.accounts.groupRouting.manage"]').trigger('click')
    const focusButton = wrapper.findAll('button')
      .find(button => button.text().includes('admin.accounts.groupRouting.setFocus'))
    expect(focusButton).toBeDefined()
    await focusButton!.trigger('click')
    await flushPromises()

    expect(updateGroup).toHaveBeenCalledWith(7, { focus_account_ids: [41, 42, 43] })
  })
})
