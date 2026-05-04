<script setup lang="ts">
import HashNode from './HashNode.vue'
import Button from './Button.vue'
import type { BatchTreeNode, GlobalLogNode } from '@/lib/api'

defineProps<{
  nodes: Array<BatchTreeNode | GlobalLogNode>
  loading?: boolean
  nextCursor?: string
  emptyLabel?: string
}>()

const emit = defineEmits<{ loadMore: [] }>()
</script>

<template>
  <div>
    <div v-if="nodes.length" class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
      <HashNode
        v-for="node in nodes"
        :key="`${node.level}:${node.start_index}`"
        :label="`L${node.level} / start ${node.start_index}`"
        :hash="node.hash"
        :meta="`width ${node.width}`"
        :active="node.width > 1"
      />
    </div>
    <p v-else class="text-[12px] text-ink-500">{{ emptyLabel || '暂无节点，调整 level/start 后重试。' }}</p>
    <div v-if="nextCursor" class="mt-4">
      <Button size="sm" variant="subtle" :loading="loading" @click="emit('loadMore')">加载更多节点</Button>
    </div>
  </div>
</template>
