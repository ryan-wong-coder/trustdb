<script setup lang="ts">
defineProps<{ state: 'ok' | 'warn' | 'bad' | 'idle'; label?: string }>()
</script>

<template>
  <span class="inline-flex items-center gap-2 text-[12px]">
    <span
      class="inline-block w-2 h-2 rounded-full shadow-[0_0_14px_currentColor]"
      :class="{
        'bg-success animate-pulseDot': state === 'ok',
        'bg-warn':                     state === 'warn',
        'bg-danger':                   state === 'bad',
        'bg-ink-400':                  state === 'idle',
      }"
    />
    <span v-if="label" class="text-ink-300">{{ label }}</span>
  </span>
</template>
