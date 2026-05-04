<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Card from '@/components/Card.vue'
import Button from '@/components/Button.vue'
import { getOverlays, proxyGet } from '@/lib/api'
import { Activity, RefreshCcw } from 'lucide-vue-next'

const overlays = ref<Record<string, unknown> | null>(null)
const latestRoot = ref<unknown>(null)
const err = ref('')
const busy = ref(false)

async function load() {
  err.value = ''
  busy.value = true
  try {
    overlays.value = await getOverlays()
  } catch (e: unknown) {
    err.value = e instanceof Error ? e.message : String(e)
    overlays.value = null
  }
  try {
    const res = await proxyGet('/v1/roots/latest')
    if (res.ok) latestRoot.value = await res.json()
    else latestRoot.value = null
  } catch {
    latestRoot.value = null
  } finally {
    busy.value = false
  }
}

onMounted(() => { load() })
</script>

<template>
  <div class="flex flex-col gap-5 max-w-[1100px] mx-auto">
    <section class="command-panel scanline rounded-[30px] p-7 animate-rise overflow-hidden">
      <div class="relative flex items-start justify-between gap-6 flex-wrap">
        <div class="min-w-[240px] flex-1">
          <p class="kicker text-[10px] font-bold">TrustDB Admin</p>
          <h1 class="display-title mt-3 text-[clamp(32px,5vw,56px)] font-black text-ink-50">运维概览</h1>
          <p class="mt-3 max-w-2xl text-[13px] text-ink-400 leading-relaxed">
            通过受保护的 Admin API 查看健康状态、监听与锚点相关配置。业务数据查询见「记录」与「指标」。
          </p>
        </div>
        <Button variant="subtle" :loading="busy" @click="load">
          <RefreshCcw :size="14" /> 刷新
        </Button>
      </div>
      <p v-if="err" class="mt-4 text-[12px] text-danger">{{ err }}</p>
    </section>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <Card title="运行覆盖" subtitle="来自 viper 的额外字段（未全部进入结构化 config）">
        <pre class="text-[11px] font-mono text-ink-300 whitespace-pre-wrap break-all">{{ JSON.stringify(overlays, null, 2) }}</pre>
      </Card>
      <Card title="最新 Batch Root" subtitle="GET /v1/roots/latest">
        <div v-if="latestRoot" class="text-[12px] font-mono text-ink-200 whitespace-pre-wrap break-all">
          {{ JSON.stringify(latestRoot, null, 2) }}
        </div>
        <div v-else class="flex items-center gap-2 text-ink-500 text-[13px]">
          <Activity :size="14" class="text-accent" /> 暂无或不可读
        </div>
      </Card>
    </div>
  </div>
</template>
