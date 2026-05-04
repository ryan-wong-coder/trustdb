<script setup lang="ts">
defineProps<{ label: string; inline?: boolean }>()
</script>

<template>
  <div :class="inline ? 'flex items-center justify-between gap-3' : ''">
    <div
      :class="[
        inline
          ? 'meta-label text-[10px] font-bold whitespace-nowrap'
          : 'meta-label text-[10px] font-bold mb-1',
      ]"
    >
      {{ label }}
    </div>
    <div class="text-[12.5px] text-ink-200 min-w-0">
      <slot />
    </div>
  </div>
</template>
