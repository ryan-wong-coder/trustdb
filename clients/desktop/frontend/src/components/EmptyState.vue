<script setup lang="ts">
defineProps<{ title: string; hint?: string; icon?: any }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center py-14 text-center animate-fadeIn">
    <div class="w-12 h-12 rounded-[18px] border border-accent/20 bg-accent/10 flex items-center justify-center mb-3 text-accent shadow-[0_0_32px_rgba(0,255,34,0.08)]">
      <component v-if="icon" :is="icon" :size="20" />
    </div>
    <h4 class="font-display text-[18px] font-black uppercase tracking-[-0.02em] text-ink-50">{{ title }}</h4>
    <p v-if="hint" class="mt-1.5 text-[12px] text-ink-400 max-w-sm leading-relaxed">{{ hint }}</p>
    <div class="mt-4">
      <slot />
    </div>
  </div>
</template>
