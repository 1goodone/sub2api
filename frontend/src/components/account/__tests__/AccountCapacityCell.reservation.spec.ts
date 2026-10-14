import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountCapacityCell from '../AccountCapacityCell.vue'
import SlotReservationSettings from '../SlotReservationSettings.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-14T00:00:00Z')) })
afterEach(() => vi.useRealTimers())

it('shows dashed reserved slots, counts down independently and removes expired occupancy', async () => {
  const now = Date.now() / 1000
  const account = { id: 1, platform: 'openai', type: 'oauth', concurrency: 3, current_concurrency: 3,
    reserved_slot_expires_at: [now + 2, now + 60] } as Account
  const wrapper = mount(AccountCapacityCell, { props: { account } })
  expect(wrapper.get('[data-testid="reserved-slots"]').text()).toContain('2s')
  expect(wrapper.get('[data-testid="reserved-slots"]').text()).toContain('60s')
  expect(wrapper.findAll('svg[stroke-dasharray="2 2"]')).toHaveLength(2)
  await vi.advanceTimersByTimeAsync(2000)
  expect(wrapper.findAll('svg[stroke-dasharray="2 2"]')).toHaveLength(1)
  expect(wrapper.get('[data-testid="reserved-slots"]').text()).toContain('58s')
  await wrapper.setProps({ account: { ...account, reserved_slot_expires_at: [], current_concurrency: 1 } })
  expect(wrapper.find('[data-testid="reserved-slots"]').exists()).toBe(false)
  wrapper.unmount()
  expect(vi.getTimerCount()).toBe(0)
})

it('uses an accessible opt-in and seconds input with a 60 second default', async () => {
  const wrapper = mount(SlotReservationSettings)
  expect(wrapper.find('input[type="number"]').exists()).toBe(false)
  await wrapper.get('input[type="checkbox"]').setValue(true)
  expect(wrapper.emitted('update:enabled')?.[0]).toEqual([true])
  expect((wrapper.get('input[type="number"]').element as HTMLInputElement).value).toBe('60')
  await wrapper.get('input[type="number"]').setValue(120)
  expect(wrapper.emitted('update:seconds')?.[0]).toEqual([120])
  wrapper.unmount()
})
