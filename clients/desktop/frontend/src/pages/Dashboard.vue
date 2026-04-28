<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api, BatchRoot, LocalRecord, Metric } from '@/lib/api'
import { useRecords } from '@/stores/records'
import { useIdentity } from '@/stores/identity'
import { useSettings } from '@/stores/settings'
import Card from '@/components/Card.vue'
import LevelBadge from '@/components/LevelBadge.vue'
import HashChip from '@/components/HashChip.vue'
import EmptyState from '@/components/EmptyState.vue'
import Button from '@/components/Button.vue'
import { bytesToHex, formatTime, relativeTime, nanoToDate } from '@/lib/format'
import { ArrowRight, CloudUpload, Activity, CircleCheckBig, FolderOpen } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'

const records = useRecords()
const identity = useIdentity()
const settings = useSettings()
const { records: recs } = storeToRefs(records)

const latestRoot = ref<BatchRoot | null>(null)
const metrics = ref<Metric[]>([])
const loadingRoot = ref(false)

async function loadServer() {
  try {
    loadingRoot.value = true
    latestRoot.value = await api.latestRoot()
  } catch (_) {
    latestRoot.value = null
  } finally {
    loadingRoot.value = false
  }
  try { metrics.value = await api.serverMetrics() } catch (_) { metrics.value = [] }
}

onMounted(async () => {
  await loadServer()
  try { await records.refreshAllPending() } catch (_) { /* ignore */ }
})

function metric(name: string): number {
  const items = metrics.value.filter((m) => m.name === name)
  if (!items.length) return 0
  return items.reduce((acc, m) => acc + m.value, 0)
}

const recent = computed<LocalRecord[]>(() => recs.value.slice(0, 5))
const counts = computed(() => ({
  total: recs.value.length,
  l3:    recs.value.filter((r) => r.proof_level === 'L3').length,
  l4:    recs.value.filter((r) => r.proof_level === 'L4').length,
  l5:    recs.value.filter((r) => r.proof_level === 'L5').length,
  pending: recs.value.filter((r) => r.proof_level !== 'L5').length,
}))
</script>

<template>
  <div class="flex flex-col gap-5 max-w-[1100px] mx-auto">
    <!-- Hero: big-picture status -->
    <section class="command-panel scanline rounded-[30px] p-7 animate-rise overflow-hidden">
      <div class="relative flex items-start justify-between gap-7 flex-wrap">
        <div class="min-w-[260px] flex-1">
          <p class="kicker text-[10px] font-bold">Current Identity</p>
          <h1 class="display-title mt-3 text-[clamp(40px,6vw,72px)] font-black text-ink-50">
            {{ identity.identity?.client_id || 'No Identity' }}
          </h1>
          <p class="mt-3 max-w-2xl text-[13px] text-ink-400 leading-relaxed">
            <template v-if="identity.identity">
              租户 <span class="text-ink-100">{{ identity.identity.tenant_id }}</span> ·
              密钥 <span class="font-mono text-accent">{{ identity.identity.key_id }}</span>
            </template>
            <template v-else>
              到 <RouterLink to="/keys" class="text-accent hover:underline">身份密钥</RouterLink> 创建或导入一个 Ed25519 身份
            </template>
          </p>
        </div>
        <div class="flex items-center gap-2">
          <RouterLink to="/attest">
            <Button><CloudUpload :size="14" /> 新建存证</Button>
          </RouterLink>
          <RouterLink to="/verify">
            <Button variant="subtle">验证证据 <ArrowRight :size="14" /></Button>
          </RouterLink>
        </div>
      </div>

      <div class="relative mt-6 grid grid-cols-2 sm:grid-cols-5 gap-3">
        <div class="surface-tile rounded-[20px] p-4">
          <div class="kicker text-[9px] font-bold">本地记录</div>
          <div class="mt-2 font-display text-[32px] num font-black text-ink-50">{{ counts.total }}</div>
        </div>
        <div class="surface-tile rounded-[20px] p-4">
          <div class="kicker text-[9px] font-bold">L3 已承诺</div>
          <div class="mt-2 font-display text-[32px] num font-black text-ink-50">{{ counts.l3 + counts.l4 + counts.l5 }}</div>
        </div>
        <div class="surface-tile rounded-[20px] p-4">
          <div class="kicker text-[9px] font-bold">L4 Global</div>
          <div class="mt-2 font-display text-[32px] num font-black text-accent">{{ counts.l4 + counts.l5 }}</div>
        </div>
        <div class="surface-tile rounded-[20px] p-4">
          <div class="kicker text-[9px] font-bold">L5 外锚</div>
          <div class="mt-2 font-display text-[32px] num font-black text-accent">{{ counts.l5 }}</div>
        </div>
        <div class="surface-tile rounded-[20px] p-4">
          <div class="kicker text-[9px] font-bold">待提升状态</div>
          <div class="mt-2 font-display text-[32px] num font-black text-ink-50">{{ counts.pending }}</div>
        </div>
      </div>
    </section>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
      <!-- Recent records -->
      <Card class="lg:col-span-2" title="最近提交" subtitle="最新五条">
        <template #actions>
          <RouterLink to="/records" class="text-[12px] text-accent hover:underline">查看全部</RouterLink>
        </template>
        <EmptyState v-if="!recent.length" title="还没有存证记录" hint="到「新建存证」拖拽一个文件即可开始。" :icon="FolderOpen" />
        <ul v-else class="divide-y hairline -my-2">
          <li v-for="r in recent" :key="r.record_id" class="py-2.5 flex items-center gap-3">
            <div class="w-8 h-8 rounded-lg bg-ink-100 dark:bg-ink-800 flex items-center justify-center text-ink-500 shrink-0">
              <CircleCheckBig :size="14" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <p class="text-[13px] font-medium text-ink-800 dark:text-ink-100 truncate max-w-[40ch]">{{ r.file_name }}</p>
                <LevelBadge :level="r.proof_level" size="sm" />
              </div>
              <p class="text-[11.5px] text-ink-500 flex items-center gap-2 mt-0.5">
                <span>{{ relativeTime(r.submitted_at) }}</span>
                <HashChip :value="r.record_id" :head="6" :tail="6" />
              </p>
            </div>
          </li>
        </ul>
      </Card>

      <!-- Server -->
      <Card title="服务器" subtitle="最新 Batch Root">
        <template #actions>
          <button class="text-[12px] text-accent hover:underline" @click="loadServer">刷新</button>
        </template>
        <div v-if="latestRoot && latestRoot.batch_id" class="space-y-2">
          <div>
            <div class="text-[10.5px] uppercase tracking-[0.08em] text-ink-500">batch_id</div>
            <HashChip :value="latestRoot.batch_id" :head="10" :tail="6" class="mt-0.5" />
          </div>
          <div>
            <div class="text-[10.5px] uppercase tracking-[0.08em] text-ink-500">batch_root (sha256)</div>
            <HashChip :value="bytesToHex(latestRoot.batch_root)" :head="10" :tail="8" class="mt-0.5" />
          </div>
          <div class="grid grid-cols-2 gap-3 pt-1">
            <div>
              <div class="text-[10.5px] uppercase tracking-[0.08em] text-ink-500">tree size</div>
              <div class="text-[13px] num font-medium">{{ latestRoot.tree_size }}</div>
            </div>
            <div>
              <div class="text-[10.5px] uppercase tracking-[0.08em] text-ink-500">closed</div>
              <div class="text-[12.5px] text-ink-700 dark:text-ink-200">{{ formatTime(nanoToDate(latestRoot.closed_at_unix_nano)) || '—' }}</div>
            </div>
          </div>
        </div>
        <EmptyState v-else title="暂无 batch root" :hint="loadingRoot ? '读取中' : ('服务器 ' + (settings.settings.server_url || '未配置') + ' 尚未产出提交过的批次')" :icon="Activity" />
      </Card>
    </div>

    <!-- Metrics snapshot -->
    <Card title="运行指标" subtitle="取自 /metrics 最新采样">
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div class="p-3 rounded-xl hairline border bg-white/50 dark:bg-ink-800/50">
          <div class="text-[10.5px] uppercase tracking-[0.08em] text-ink-500">claims accepted</div>
          <div class="mt-1 text-[20px] num font-semibold">{{ metric('trustdb_ingest_accepted_total').toFixed(0) }}</div>
        </div>
        <div class="p-3 rounded-xl hairline border bg-white/50 dark:bg-ink-800/50">
          <div class="text-[10.5px] uppercase tracking-[0.08em] text-ink-500">batches committed</div>
          <div class="mt-1 text-[20px] num font-semibold">{{ metric('trustdb_batch_committed_total').toFixed(0) }}</div>
        </div>
        <div class="p-3 rounded-xl hairline border bg-white/50 dark:bg-ink-800/50">
          <div class="text-[10.5px] uppercase tracking-[0.08em] text-ink-500">anchors published</div>
          <div class="mt-1 text-[20px] num font-semibold">{{ metric('trustdb_anchor_published_total').toFixed(0) }}</div>
        </div>
        <div class="p-3 rounded-xl hairline border bg-white/50 dark:bg-ink-800/50">
          <div class="text-[10.5px] uppercase tracking-[0.08em] text-ink-500">anchor pending</div>
          <div class="mt-1 text-[20px] num font-semibold">{{ metric('trustdb_anchor_pending').toFixed(0) }}</div>
        </div>
      </div>
    </Card>
  </div>
</template>
