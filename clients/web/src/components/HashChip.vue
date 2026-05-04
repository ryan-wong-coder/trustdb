<script setup lang="ts">
import { computed } from 'vue'
import { Copy, Check } from 'lucide-vue-next'
import { ref } from 'vue'
import { bytesToHex, copyToClipboard, shortHash, type BytesLike } from '@/lib/format'

const props = defineProps<{
  value?: BytesLike
  mono?: boolean
  full?: boolean
  label?: string
  head?: number
  tail?: number
}>()

const copied = ref(false)
const normalized = computed(() => {
  if (typeof props.value === 'string') return props.value
  return bytesToHex(props.value)
})
const display = computed(() => {
  if (!normalized.value) return '—'
  if (props.full) return normalized.value
  return shortHash(normalized.value, props.head ?? 8, props.tail ?? 6)
})

async function copy() {
  if (!normalized.value) return
  await copyToClipboard(normalized.value)
  copied.value = true
  setTimeout(() => (copied.value = false), 1200)
}
</script>

<template>
  <button
    type="button"
    class="group inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 border border-white/10 bg-[#070807]/70 transition hover:border-accent/45 hover:bg-accent/10"
    :title="normalized"
    @click="copy"
  >
    <span v-if="label" class="font-display text-[10px] uppercase tracking-[0.12em] text-ink-400">{{ label }}</span>
    <span class="font-mono text-[11.5px] text-ink-100">{{ display }}</span>
    <Copy v-if="!copied" :size="11" class="text-ink-500 group-hover:text-accent transition" />
    <Check v-else :size="11" class="text-success" />
  </button>
</template>
