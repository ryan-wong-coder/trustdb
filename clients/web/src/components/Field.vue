<script setup lang="ts">
defineProps<{ label?: string; hint?: string }>()
</script>

<template>
  <label class="block">
    <span v-if="label" class="meta-label block text-[10.5px] font-bold mb-1.5">{{ label }}</span>
    <slot />
    <span v-if="hint" class="block mt-1.5 text-[11.5px] text-ink-400 leading-relaxed">{{ hint }}</span>
  </label>
</template>
