import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import GlobalTree from './GlobalTree.vue'

describe('GlobalTree page', () => {
  it('loads global tree summary, nodes and leaves', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true, state: { tree_size: 2, root_hash: [9] }, sth: { tree_size: 2, root_hash: [9] } }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ leaves: [{ schema_version: 'l', batch_id: 'batch-a', leaf_index: 0, batch_tree_size: 2, leaf_hash: [1] }] }), { status: 200 })))

    const wrapper = mount(GlobalTree)
    await flushPromises()

    expect(wrapper.text()).toContain('tree size')
    expect(wrapper.text()).toContain('batch-a')
    expect(wrapper.text()).toContain('L1 / start 0')
    vi.unstubAllGlobals()
  })
})
