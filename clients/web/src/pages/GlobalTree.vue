<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Card from '@/components/Card.vue'
import Button from '@/components/Button.vue'
import HashChip from '@/components/HashChip.vue'
import MerkleTreeExplorer from '@/components/MerkleTreeExplorer.vue'
import { getGlobalLeaves, getGlobalTree, getGlobalTreeNodes, type GlobalLogLeaf, type GlobalLogNode, type GlobalLogState, type SignedTreeHead } from '@/lib/api'
import { formatTime, nanoToDate } from '@/lib/format'
import { RefreshCcw } from 'lucide-vue-next'

type ExpandableNode = Pick<GlobalLogNode, 'level' | 'start_index' | 'width'>

const state = ref<GlobalLogState | null>(null)
const sth = ref<SignedTreeHead | null>(null)
const nodes = ref<GlobalLogNode[]>([])
const leaves = ref<GlobalLogLeaf[]>([])
const nodeCursor = ref('')
const leafCursor = ref('')
const level = ref<number | undefined>()
const start = ref<number | undefined>()
const loading = ref(false)
const err = ref('')

const treeSize = computed(() => state.value?.tree_size ?? sth.value?.tree_size ?? 0)
const rootHash = computed(() => state.value?.root_hash || sth.value?.root_hash || [])

async function loadSummary() {
  const body = await getGlobalTree()
  state.value = body.state ?? null
  sth.value = body.sth ?? null
}

async function loadNodes(reset: boolean) {
  if (reset) nodeCursor.value = ''
  const body = await getGlobalTreeNodes({ level: level.value, start: start.value, limit: 40, cursor: nodeCursor.value })
  nodes.value = reset ? body.nodes : nodes.value.concat(body.nodes)
  nodeCursor.value = body.next_cursor ?? ''
}

function rangeLevel(size: number): number {
  if (size <= 1) return 0
  let level = 0
  let width = 1
  while (width < size) {
    width <<= 1
    level += 1
  }
  return level
}

function largestPowerOfTwoLessThan(size: number): number {
  let out = 1
  while (out << 1 < size) out <<= 1
  return out
}

function isPowerOfTwo(size: number): boolean {
  return size > 0 && 2 ** Math.floor(Math.log2(size)) === size
}

function syntheticNode(level: number, start: number, width: number, hash: GlobalLogNode['hash'] = []): GlobalLogNode {
  return {
    schema_version: 'synthetic-global-node',
    level,
    start_index: start,
    width,
    hash,
    created_at_unix_nano: 0,
  }
}

function mergeNodes(incoming: GlobalLogNode[]) {
  const map = new Map(nodes.value.map((node) => [`${node.level}:${node.start_index}:${node.width}`, node]))
  for (const node of incoming) map.set(`${node.level}:${node.start_index}:${node.width}`, node)
  nodes.value = [...map.values()]
}

async function loadRootNode() {
  if (treeSize.value <= 0) {
    nodes.value = []
    nodeCursor.value = ''
    return
  }
  nodes.value = [syntheticNode(rangeLevel(treeSize.value), 0, treeSize.value, rootHash.value)]
  nodeCursor.value = ''
}

async function loadRangeNode(start: number, width: number): Promise<GlobalLogNode> {
  const childLevel = rangeLevel(width)
  if (isPowerOfTwo(width)) {
    const body = await getGlobalTreeNodes({ level: childLevel, start, limit: 1 })
    if (body.nodes[0]) return body.nodes[0]
  }
  return syntheticNode(childLevel, start, width)
}

async function expandNode(node: ExpandableNode) {
  if (node.width <= 1) return
  loading.value = true
  err.value = ''
  try {
    const leftWidth = largestPowerOfTwoLessThan(node.width)
    const rightWidth = node.width - leftWidth
    const [left, right] = await Promise.all([
      loadRangeNode(node.start_index, leftWidth),
      loadRangeNode(node.start_index + leftWidth, rightWidth),
    ])
    mergeNodes([left, right])
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

async function loadLeaves(reset: boolean) {
  if (reset) leafCursor.value = ''
  const body = await getGlobalLeaves({ limit: 30, cursor: leafCursor.value })
  leaves.value = reset ? body.leaves : leaves.value.concat(body.leaves)
  leafCursor.value = body.next_cursor ?? ''
}

async function refreshAll() {
  loading.value = true
  err.value = ''
  try {
    await loadSummary()
      await loadRootNode()
    await loadLeaves(true)
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

onMounted(refreshAll)
</script>

<template>
  <div class="flex flex-col gap-4 max-w-[1280px] mx-auto">
    <Card title="全局 Merkle Tree" subtitle="Global transparency log，按节点和 leaf 分页浏览">
      <template #actions>
        <Button size="sm" variant="subtle" :loading="loading" @click="refreshAll">
          <RefreshCcw :size="12" /> 刷新
        </Button>
      </template>
      <p v-if="err" class="mb-3 text-[12px] text-danger">{{ err }}</p>
      <div class="grid grid-cols-1 lg:grid-cols-3 gap-3 text-[12px]">
        <div class="rounded-[14px] bg-white/[0.03] border border-white/10 p-3">
          <div class="kicker text-[9px]">tree size</div>
          <div class="mt-1 font-mono text-accent">{{ state?.tree_size ?? '—' }}</div>
        </div>
        <div class="rounded-[14px] bg-white/[0.03] border border-white/10 p-3">
          <div class="kicker text-[9px]">latest STH</div>
          <div class="mt-1 text-ink-300">{{ formatTime(nanoToDate(sth?.timestamp_unix_nano)) || '—' }}</div>
        </div>
        <div class="rounded-[14px] bg-white/[0.03] border border-white/10 p-3">
          <div class="kicker text-[9px]">root</div>
          <div class="mt-1"><HashChip :value="sth?.root_hash || state?.root_hash" /></div>
        </div>
      </div>
    </Card>

    <div class="grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_380px] gap-4">
      <Card title="全局节点" subtitle="读取已持久化的 GlobalLogNode，不重建整棵树">
        <div class="flex flex-wrap gap-2 mb-4">
          <input v-model.number="level" type="number" min="0" class="h-9 w-[130px] px-3 rounded-[12px] border border-white/10 bg-[#0b0d0b]/80 text-[12px] text-ink-100" placeholder="可选 level" />
          <input v-model.number="start" type="number" min="0" class="h-9 w-[130px] px-3 rounded-[12px] border border-white/10 bg-[#0b0d0b]/80 text-[12px] text-ink-100" placeholder="可选 start" />
          <Button size="sm" variant="subtle" :loading="loading" @click="loadNodes(true)">读取节点</Button>
        </div>
        <MerkleTreeExplorer
          :nodes="nodes"
          :tree-size="treeSize"
          :next-cursor="nodeCursor"
          :loading="loading"
          @load-more="loadNodes(false)"
          @expand="expandNode"
        />
      </Card>

      <Card title="全局 leaves" subtitle="每个 leaf 对应一个 committed batch">
        <div class="space-y-2 max-h-[620px] overflow-y-auto pr-1">
          <div v-for="leaf in leaves" :key="leaf.leaf_index" class="rounded-[12px] border border-white/10 bg-[#070807]/80 p-3">
            <div class="flex justify-between gap-2 text-[11px] text-ink-400">
              <span>#{{ leaf.leaf_index }}</span>
              <span>batch size {{ leaf.batch_tree_size }}</span>
            </div>
            <div class="mt-1 text-[11px] font-mono text-ink-300 break-all">{{ leaf.batch_id }}</div>
            <HashChip class="mt-2" :value="leaf.leaf_hash" />
          </div>
        </div>
        <Button v-if="leafCursor" class="mt-3" size="sm" variant="subtle" :loading="loading" @click="loadLeaves(false)">加载更多 leaves</Button>
      </Card>
    </div>
  </div>
</template>
