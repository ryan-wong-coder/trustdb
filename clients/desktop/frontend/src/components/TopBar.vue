<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { RefreshCw, Wifi } from 'lucide-vue-next'
import { api, HealthStatus } from '@/lib/api'
import StatusDot from './StatusDot.vue'

const route = useRoute()
const health = ref<HealthStatus | null>(null)
const pinging = ref(false)
let timer: number | undefined

async function ping() {
  pinging.value = true
  try { health.value = await api.serverHealth() }
  finally { pinging.value = false }
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
  <header class="relative z-[2] h-[58px] drag-region hairline-b flex items-center justify-between px-6 glass">
    <div class="no-drag flex items-center gap-3">
      <span class="inline-block h-2 w-2 rounded-full bg-accent shadow-[0_0_18px_rgba(0,255,34,0.9)]"></span>
      <div>
        <div class="kicker text-[9px] font-bold">Command Surface</div>
        <div class="font-display text-[18px] font-black uppercase tracking-[-0.025em] text-ink-50">
          {{ route.meta.title ?? 'TrustDB' }}
        </div>
      </div>
    </div>
    <div class="no-drag flex items-center gap-3">
      <span v-if="health?.ok" class="inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/5 px-2.5 py-1 font-mono text-[11px] text-ink-300">
        <Wifi :size="12" class="text-accent" />
        {{ health.rtt_millis }} ms
      </span>
      <div class="rounded-full border border-white/10 bg-[#070807]/70 px-3 py-1">
        <StatusDot
          v-if="health"
          :state="health.ok ? 'ok' : 'bad'"
          :label="health.ok ? '服务在线' : (health.error || '不可达')"
        />
        <StatusDot v-else state="idle" label="检测中" />
      </div>
      <button
        class="no-drag rounded-full border border-white/10 bg-white/5 p-2 text-ink-400 transition hover:border-accent/45 hover:text-accent"
        :class="{ 'animate-spin': pinging }"
        @click="ping"
        title="刷新"
      >
        <RefreshCw :size="13" />
      </button>
    </div>
  </header>
</template>
