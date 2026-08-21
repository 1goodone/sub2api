import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountGroupsCell from '../AccountGroupsCell.vue'
import type { AdminGroup, Group } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const rowGroup = {
  id: 7,
  name: 'Primary',
  platform: 'openai',
  subscription_type: 'standard',
  rate_multiplier: 1
} as unknown as Group

const routingGroup = {
  ...rowGroup,
  focus_account_id: 42,
  focus_account_ids: [42, 43]
} as unknown as AdminGroup

describe('AccountGroupsCell routing controls', () => {
  it('shows and updates membership in a multi-account focus pool', async () => {
    const wrapper = mount(AccountGroupsCell, {
      props: {
        groups: [rowGroup],
        routingGroups: [routingGroup],
        accountId: 42
      },
      global: {
        stubs: {
          Teleport: true,
          GroupBadge: { template: '<span>{{ name }}</span>', props: ['name'] }
        }
      }
    })

    await wrapper.get('button[title="admin.accounts.groupRouting.manage"]').trigger('click')
    expect(wrapper.text()).toContain('admin.accounts.groupRouting.focused')

    const buttons = wrapper.findAll('button')
    await buttons.find(button => button.text().includes('admin.accounts.groupRouting.clearFocus'))!.trigger('click')
    expect(wrapper.emitted('toggleFocus')).toEqual([[7]])
    expect(wrapper.text()).not.toContain('admin.groups.accountRouting.authFirst')
    expect(wrapper.text()).not.toContain('admin.groups.accountRouting.apiFirst')
  })

  it('shows an unfocused account without clearing other pool members', async () => {
    const wrapper = mount(AccountGroupsCell, {
      props: {
        groups: [rowGroup],
        routingGroups: [routingGroup],
        accountId: 44
      },
      global: {
        stubs: {
          Teleport: true,
          GroupBadge: { template: '<span>{{ name }}</span>', props: ['name'] }
        }
      }
    })

    await wrapper.get('button[title="admin.accounts.groupRouting.manage"]').trigger('click')
    expect(wrapper.text()).not.toContain('admin.accounts.groupRouting.focused')
    expect(wrapper.text()).toContain('admin.accounts.groupRouting.setFocus')
  })
})
