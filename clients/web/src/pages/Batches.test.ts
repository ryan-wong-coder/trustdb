import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import Batches from './Batches.vue'

vi.mock('vue-router', () => ({
  RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
}))

describe('Batches page', () => {
  it('loads and displays batch history', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      roots: [{ batch_id: 'batch-a', tree_size: 2, closed_at_unix_nano: 1000, batch_root: [9] }],
    }), { status: 200 })))

    const wrapper = mount(Batches)
    await flushPromises()

    expect(wrapper.text()).toContain('batch-a')
    expect(wrapper.text()).toContain('2')
    vi.unstubAllGlobals()
  })
})
