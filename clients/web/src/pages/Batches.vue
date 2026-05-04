<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import Card from '@/components/Card.vue'
import Button from '@/components/Button.vue'
import HashChip from '@/components/HashChip.vue'
import EmptyState from '@/components/EmptyState.vue'
import { getBatches, type BatchRoot } from '@/lib/api'
import { formatTime, nanoToDate } from '@/lib/format'
import { GitBranch, RefreshCcw } from 'lucide-vue-next'

const roots = ref<BatchRoot[]>([])
const cursor = ref('')
const loading = ref(false)
const err = ref('')

async function load(reset: boolean) {
  if (reset) cursor.value = ''
  loading.value = true
  err.value = ''
  try {
    const body = await getBatches({ limit: 50, cursor: cursor.value })
    roots.value = reset ? body.roots : roots.value.concat(body.roots)
    cursor.value = body.next_cursor ?? ''
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

onMounted(() => load(true))
</script>

<template>
  <div class="flex flex-col gap-4 max-w-[1200px] mx-auto">
    <Card title="历史批次" subtitle="GET /v1/batches，按 cursor 分页读取 batch roots">
      <template #actions>
        <Button size="sm" variant="subtle" :loading="loading" @click="load(true)">
          <RefreshCcw :size="12" /> 刷新
        </Button>
      </template>
      <p v-if="err" class="mb-3 text-[12px] text-danger">{{ err }}</p>
      <EmptyState v-if="!roots.length && !loading" title="暂无批次" hint="提交记录后会生成 batch root。" :icon="GitBranch" />
      <div v-else class="overflow-x-auto">
        <table class="w-full text-[12px]">
          <thead class="text-ink-500 text-left">
            <tr>
              <th class="py-2 font-normal">batch</th>
              <th class="px-2 py-2 font-normal">tree size</th>
              <th class="px-2 py-2 font-normal">closed</th>
              <th class="px-2 py-2 font-normal">root</th>
              <th class="py-2 font-normal text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="root in roots" :key="root.batch_id" class="border-t border-[var(--hairline)]">
              <td class="py-2"><HashChip :value="root.batch_id" :head="18" :tail="8" /></td>
              <td class="px-2 py-2 text-ink-300 font-mono">{{ root.tree_size }}</td>
              <td class="px-2 py-2 text-ink-400">{{ formatTime(nanoToDate(root.closed_at_unix_nano)) }}</td>
              <td class="px-2 py-2"><HashChip :value="root.batch_root" :head="10" :tail="8" /></td>
              <td class="py-2 text-right">
                <RouterLink class="text-accent hover:underline" :to="`/batches/${encodeURIComponent(root.batch_id)}`">查看树</RouterLink>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="cursor" class="mt-4">
        <Button size="sm" variant="subtle" :loading="loading" @click="load(false)">加载更多</Button>
      </div>
    </Card>
  </div>
</template>
