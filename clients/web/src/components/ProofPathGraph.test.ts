import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ProofPathGraph from './ProofPathGraph.vue'

describe('ProofPathGraph', () => {
  it('renders leaf, siblings and batch root', () => {
    const wrapper = mount(ProofPathGraph, {
      props: {
        proof: {
          record_id: 'rec-a',
          proof_level: 'L5',
          proof_bundle: {
            record_id: 'rec-a',
            committed_receipt: {
              batch_id: 'batch-a',
              leaf_index: 0,
              leaf_hash: [1],
              batch_root: [9],
              batch_closed_at_unix_nano: 100,
            },
            batch_proof: {
              tree_alg: 'rfc6962-sha256',
              leaf_index: 0,
              tree_size: 2,
              audit_path: [[2]],
            },
          },
        },
      },
    })

    expect(wrapper.text()).toContain('Leaf')
    expect(wrapper.text()).toContain('Sibling 1')
    expect(wrapper.text()).toContain('Batch Root')
  })
})
