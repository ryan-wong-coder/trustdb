<script setup lang="ts">
defineProps<{
  title?: string
  subtitle?: string
  dense?: boolean
}>()
</script>

<template>
  <section class="glass rounded-xl2 animate-fadeIn overflow-hidden">
    <header v-if="title || $slots.title || $slots.actions" class="flex items-center justify-between gap-4 px-5 pt-4 pb-3 hairline-b">
      <div>
        <slot name="title">
          <h3 class="font-display text-[17px] font-bold uppercase tracking-[0.03em] text-ink-50">{{ title }}</h3>
        </slot>
        <p v-if="subtitle" class="mt-1 text-[12px] text-ink-400 leading-relaxed">{{ subtitle }}</p>
      </div>
      <div class="flex items-center gap-2">
        <slot name="actions" />
      </div>
    </header>
    <div :class="[dense ? 'p-0' : 'px-5 py-4']">
      <slot />
    </div>
  </section>
</template>
