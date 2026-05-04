<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import Card from '@/components/Card.vue'
import Button from '@/components/Button.vue'
import HashChip from '@/components/HashChip.vue'
import MerkleTreeExplorer from '@/components/MerkleTreeExplorer.vue'
import ProofPathGraph from '@/components/ProofPathGraph.vue'
import { getBatchDetail, getBatchLeaves, getBatchTreeNodes, getProofPath, type BatchTreeLeaf, type BatchTreeNode, type ProofResponse } from '@/lib/api'
import { formatTime, nanoToDate } from '@/lib/format'
import { RefreshCcw } from 'lucide-vue-next'

const route = useRoute()
const batchID = String(route.params.batchID || '')
const detail = ref<Awaited<ReturnType<typeof getBatchDetail>> | null>(null)
const leaves = ref<BatchTreeLeaf[]>([])
const nodes = ref<BatchTreeNode[]>([])
const nodeCursor = ref('')
const leafCursor = ref('')
const level = ref(0)
const start = ref(0)
const recordID = ref('')
const proof = ref<ProofResponse | null>(null)
const loading = ref(false)
const err = ref('')

async function loadSummary() {
  detail.value = await getBatchDetail(batchID)
}

async function loadLeaves(reset: boolean) {
  if (reset) leafCursor.value = ''
  const body = await getBatchLeaves(batchID, { limit: 25, cursor: leafCursor.value })
  leaves.value = reset ? body.leaves : leaves.value.concat(body.leaves)
  leafCursor.value = body.next_cursor ?? ''
  if (!recordID.value && leaves.value[0]) recordID.value = leaves.value[0].record_id
}

async function loadNodes(reset: boolean) {
  if (reset) nodeCursor.value = ''
  const body = await getBatchTreeNodes(batchID, { level: level.value, start: start.value, limit: 30, cursor: nodeCursor.value })
  nodes.value = reset ? body.nodes : nodes.value.concat(body.nodes)
  nodeCursor.value = body.next_cursor ?? ''
}

async function loadProof() {
  if (!recordID.value.trim()) return
  const body = await getProofPath(recordID.value.trim())
  if (body.proof_bundle.committed_receipt.batch_id !== batchID) {
    throw new Error('该 record_id 不属于当前批次')
  }
  proof.value = body
}

async function refreshAll() {
  loading.value = true
  err.value = ''
  try {
    await loadSummary()
    await loadLeaves(true)
    await loadNodes(true)
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

async function submitProof() {
  loading.value = true
  err.value = ''
  try {
    await loadProof()
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
    <Card title="批次详情" :subtitle="batchID">
      <template #actions>
        <Button size="sm" variant="subtle" :loading="loading" @click="refreshAll">
          <RefreshCcw :size="12" /> 刷新
        </Button>
      </template>
      <p v-if="err" class="mb-3 text-[12px] text-danger">{{ err }}</p>
      <div v-if="detail" class="grid grid-cols-1 lg:grid-cols-4 gap-3 text-[12px]">
        <div class="rounded-[14px] bg-white/[0.03] border border-white/10 p-3">
          <div class="kicker text-[9px]">records</div>
          <div class="mt-1 font-mono text-accent">{{ detail.record_count }}</div>
        </div>
        <div class="rounded-[14px] bg-white/[0.03] border border-white/10 p-3">
          <div class="kicker text-[9px]">tree size</div>
          <div class="mt-1 font-mono text-ink-100">{{ detail.root.tree_size }}</div>
        </div>
        <div class="rounded-[14px] bg-white/[0.03] border border-white/10 p-3">
          <div class="kicker text-[9px]">closed</div>
          <div class="mt-1 text-ink-300">{{ formatTime(nanoToDate(detail.root.closed_at_unix_nano)) }}</div>
        </div>
        <div class="rounded-[14px] bg-white/[0.03] border border-white/10 p-3">
          <div class="kicker text-[9px]">root</div>
          <div class="mt-1"><HashChip :value="detail.root.batch_root" /></div>
        </div>
      </div>
    </Card>

    <div class="grid grid-cols-1 xl:grid-cols-[360px_minmax(0,1fr)] gap-4">
      <Card title="批次叶子" subtitle="按需分页，不展开全部记录">
        <div class="space-y-2 max-h-[560px] overflow-y-auto pr-1">
          <button
            v-for="leaf in leaves"
            :key="leaf.leaf_index"
            type="button"
            class="w-full text-left rounded-[12px] border border-white/10 bg-[#070807]/80 p-3 hover:border-accent/50 transition"
            @click="recordID = leaf.record_id"
          >
            <div class="flex justify-between gap-2 text-[11px] text-ink-400">
              <span>#{{ leaf.leaf_index }}</span>
              <span>{{ leaf.record_id }}</span>
            </div>
            <HashChip class="mt-2" :value="leaf.leaf_hash" />
          </button>
        </div>
        <Button v-if="leafCursor" class="mt-3" size="sm" variant="subtle" :loading="loading" @click="loadLeaves(false)">加载更多叶子</Button>
      </Card>

      <div class="flex flex-col gap-4">
        <Card title="Proof Path" subtitle="单条记录从 leaf 到 batch root 的审计路径">
          <div class="flex flex-wrap gap-2 mb-4">
            <input v-model="recordID" class="h-9 min-w-[280px] flex-1 px-3 rounded-[12px] border border-white/10 bg-[#0b0d0b]/80 text-[12px] text-ink-100" placeholder="record_id" @keyup.enter="submitProof" />
            <Button size="sm" :loading="loading" @click="submitProof">查看 proof path</Button>
          </div>
          <ProofPathGraph :proof="proof" />
        </Card>

        <Card title="Tree Explorer" subtitle="按 level/start 分页读取真实批次 Merkle 节点">
          <div class="flex flex-wrap gap-2 mb-4">
            <input v-model.number="level" type="number" min="0" class="h-9 w-[110px] px-3 rounded-[12px] border border-white/10 bg-[#0b0d0b]/80 text-[12px] text-ink-100" placeholder="level" />
            <input v-model.number="start" type="number" min="0" class="h-9 w-[130px] px-3 rounded-[12px] border border-white/10 bg-[#0b0d0b]/80 text-[12px] text-ink-100" placeholder="start" />
            <Button size="sm" variant="subtle" :loading="loading" @click="loadNodes(true)">读取节点</Button>
          </div>
          <MerkleTreeExplorer :nodes="nodes" :next-cursor="nodeCursor" :loading="loading" @load-more="loadNodes(false)" />
        </Card>
      </div>
    </div>
  </div>
</template>
