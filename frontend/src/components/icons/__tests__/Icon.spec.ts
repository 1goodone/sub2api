import { afterEach, expect, it } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import Icon from '../Icon.vue'

enableAutoUnmount(afterEach)

it('renders SVG geometry and updates it when the icon changes', async () => {
  const wrapper = mount(Icon, { props: { name: 'search' } })
  expect(wrapper.get('circle').attributes()).toMatchObject({ cx: '11', cy: '11', r: '8' })
  expect(wrapper.get('path').element.namespaceURI).toBe('http://www.w3.org/2000/svg')
  expect(wrapper.get('path').attributes('d')).not.toContain('[object Object]')

  await wrapper.setProps({ name: 'grid' })
  expect(wrapper.find('circle').exists()).toBe(false)
  expect(wrapper.findAll('rect')).toHaveLength(4)
  expect(wrapper.get('rect').element.namespaceURI).toBe('http://www.w3.org/2000/svg')
  expect(wrapper.get('svg').attributes('aria-hidden')).toBe('true')
})
