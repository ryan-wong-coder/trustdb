<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { RefreshCw, Wifi } from 'lucide-vue-next'
import { proxyGet } from '@/lib/api'
import StatusDot from './StatusDot.vue'

const route = useRoute()
const healthOk = ref<boolean | null>(null)
const pinging = ref(false)
let timer: number | undefined

async function ping() {
  pinging.value = true
  try {
    const res = await proxyGet('/healthz')
    if (!res.ok) {
      healthOk.value = false
      return
    }
    const j = (await res.json()) as { ok?: boolean }
    healthOk.value = !!j.ok
  } catch {
    healthOk.value = false
  } finally {
    pinging.value = false
  }
}

onMounted(() => {
  ping()
  timer = window.setInterval(ping, 15_000)
})
onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <header class="relative z-[2] h-[58px] hairline-b flex items-center justify-between px-6 glass">
    <div class="flex items-center gap-3">
      <span class="inline-block h-2 w-2 rounded-full bg-accent shadow-[0_0_18px_rgba(0,255,34,0.9)]"></span>
      <div>
        <div class="kicker text-[9px] font-bold">Admin Surface</div>
        <div class="font-display text-[18px] font-black uppercase tracking-[-0.025em] text-ink-50">
          {{ route.meta.title ?? 'TrustDB' }}
        </div>
      </div>
    </div>
    <div class="flex items-center gap-3">
      <span v-if="healthOk" class="inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/5 px-2.5 py-1 font-mono text-[11px] text-ink-300">
        <Wifi :size="12" class="text-accent" />
        HTTP
      </span>
      <div class="rounded-full border border-white/10 bg-[#070807]/70 px-3 py-1">
        <StatusDot
          v-if="healthOk !== null"
          :state="healthOk ? 'ok' : 'bad'"
          :label="healthOk ? '服务在线' : '不可达'"
        />
        <StatusDot v-else state="idle" label="检测中" />
      </div>
      <button
        class="rounded-full border border-white/10 bg-white/5 p-2 text-ink-400 transition hover:border-accent/45 hover:text-accent"
        :class="{ 'animate-spin': pinging }"
        type="button"
        title="刷新"
        @click="ping"
      >
        <RefreshCw :size="13" />
      </button>
    </div>
  </header>
</template>
