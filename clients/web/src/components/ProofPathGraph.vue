<script setup lang="ts">
import HashNode from './HashNode.vue'
import type { ProofResponse } from '@/lib/api'

defineProps<{ proof: ProofResponse | null }>()
</script>

<template>
  <div v-if="proof" class="overflow-x-auto">
    <div class="flex items-center gap-3 min-w-max py-2">
      <HashNode
        label="Leaf"
        :hash="proof.proof_bundle.committed_receipt.leaf_hash"
        :meta="`#${proof.proof_bundle.committed_receipt.leaf_index}`"
        active
      />
      <template v-for="(hash, i) in proof.proof_bundle.batch_proof.audit_path" :key="i">
        <span class="text-ink-600">→</span>
        <HashNode :label="`Sibling ${i + 1}`" :hash="hash" :meta="`path[${i}]`" />
      </template>
      <span class="text-ink-600">→</span>
      <HashNode label="Batch Root" :hash="proof.proof_bundle.committed_receipt.batch_root" :meta="`size ${proof.proof_bundle.batch_proof.tree_size}`" active />
    </div>
  </div>
  <p v-else class="text-[12px] text-ink-500">输入 record_id 后查看从 leaf 到 batch root 的 proof path。</p>
</template>
