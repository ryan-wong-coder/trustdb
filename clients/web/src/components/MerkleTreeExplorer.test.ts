import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MerkleTreeExplorer from './MerkleTreeExplorer.vue'

describe('MerkleTreeExplorer', () => {
  it('renders paged nodes and emits loadMore', async () => {
    const wrapper = mount(MerkleTreeExplorer, {
      props: {
        nodes: [{ schema_version: 'x', batch_id: 'b1', level: 1, start_index: 0, width: 2, hash: [1, 2, 3] }],
        nextCursor: 'cursor',
      },
    })

    expect(wrapper.text()).toContain('L1 / start 0')
    expect(wrapper.text()).toContain('width 2')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('loadMore')).toHaveLength(1)
  })

  it('renders empty state for missing nodes', () => {
    const wrapper = mount(MerkleTreeExplorer, { props: { nodes: [], emptyLabel: '暂无树节点' } })
    expect(wrapper.text()).toContain('暂无树节点')
  })
})
