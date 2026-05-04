<script setup lang="ts">
import { bytesToHex, shortHash, type BytesLike } from '@/lib/format'

const props = defineProps<{
  label: string
  hash?: BytesLike
  meta?: string
  active?: boolean
}>()

const hex = () => bytesToHex(props.hash)
</script>

<template>
  <div
    class="rounded-[14px] border px-3 py-2 bg-[#080a08]/80 min-w-[180px]"
    :class="active ? 'border-accent/70 shadow-acid' : 'border-white/10'"
  >
    <div class="flex items-center justify-between gap-3">
      <span class="text-[11px] font-bold text-ink-200">{{ label }}</span>
      <span v-if="meta" class="text-[10px] text-ink-500">{{ meta }}</span>
    </div>
    <div class="mt-1 font-mono text-[11px] text-accent" :title="hex()">
      {{ shortHash(hex(), 10, 8) || '—' }}
    </div>
  </div>
</template>
