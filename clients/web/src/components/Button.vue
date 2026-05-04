<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  variant?: 'primary' | 'ghost' | 'subtle' | 'danger'
  size?: 'sm' | 'md'
  loading?: boolean
  disabled?: boolean
  type?: 'button' | 'submit' | 'reset'
}>()

const variantClass = computed(() => {
  switch (props.variant ?? 'primary') {
    case 'ghost':
      return 'bg-transparent text-ink-200 hover:text-accent hover:bg-accent/10'
    case 'subtle':
      return 'bg-panel-raised/70 text-ink-100 hover:text-accent border border-white/10 hover:border-accent/35 shadow-soft-sm'
    case 'danger':
      return 'bg-danger text-[#130500] hover:bg-danger/90 border border-danger/40 shadow-[0_0_28px_rgba(255,77,0,0.22)]'
    default:
      return 'bg-accent text-[#041105] hover:bg-accent-300 active:bg-accent-600 border border-accent/40 shadow-acid'
  }
})

const sizeClass = computed(() => (props.size === 'sm' ? 'h-8 px-3 text-[11px]' : 'h-10 px-4 text-[12px]'))
</script>

<template>
  <button
    :type="type ?? 'button'"
    :disabled="disabled || loading"
    class="inline-flex items-center justify-center gap-1.5 rounded-[12px] font-display font-bold uppercase tracking-[0.13em] transition-all duration-150 ease-ios disabled:opacity-45 disabled:cursor-not-allowed whitespace-nowrap select-none"
    :class="[variantClass, sizeClass]"
  >
    <span v-if="loading" class="inline-block w-3 h-3 rounded-full border-2 border-current border-b-transparent animate-spin" />
    <slot />
  </button>
</template>
