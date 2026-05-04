<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ level?: string; size?: 'sm' | 'md' }>()

const PALETTE: Record<string, { fg: string; bg: string; label: string }> = {
  L1: { fg: 'text-ink-300',   bg: 'bg-ink-700/80 border-white/10',      label: 'L1 LOCAL' },
  L2: { fg: 'text-[#d7ff3f]', bg: 'bg-[#d7ff3f]/10 border-[#d7ff3f]/25', label: 'L2 RECEIVED' },
  L3: { fg: 'text-accent',    bg: 'bg-accent/10 border-accent/25',       label: 'L3 BATCH' },
  L4: { fg: 'text-accent',    bg: 'bg-accent/15 border-accent/40',       label: 'L4 GLOBAL' },
  L5: { fg: 'text-[#041105]', bg: 'bg-accent border-accent/70',          label: 'L5 NOTARIZED' },
}

const info = computed(() => PALETTE[props.level ?? ''] ?? { fg: 'text-ink-400', bg: 'bg-ink-800/70 border-white/10', label: props.level ?? '—' })
const padding = computed(() => (props.size === 'sm' ? 'h-5 px-1.5 text-[9.5px]' : 'h-6 px-2.5 text-[10px]'))
</script>

<template>
  <span
    class="inline-flex items-center rounded-full border font-display font-bold tabular-nums uppercase tracking-[0.11em] shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]"
    :class="[info.fg, info.bg, padding]"
  >
    {{ info.label }}
  </span>
</template>
