<script setup lang="ts">
import { onMounted, onUnmounted, watch } from 'vue'

const props = defineProps<{ open: boolean; title?: string; width?: string }>()
const emit = defineEmits<{ (e: 'close'): void }>()

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.open) emit('close')
}

onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))

watch(
  () => props.open,
  (v) => {
    document.documentElement.style.overflow = v ? 'hidden' : ''
  },
)
</script>

<template>
  <teleport to="body">
    <transition name="drawer">
      <div v-if="open" class="fixed inset-0 z-40">
        <div class="absolute inset-0 bg-black/58 backdrop-blur-[5px]" @click="emit('close')" />
        <aside
          class="absolute right-0 top-0 h-full glass shadow-soft-lg flex flex-col overflow-hidden"
          :style="{ width: width ?? '520px' }"
        >
          <header class="flex items-center justify-between px-5 h-12 hairline-b">
            <h3 class="font-display text-[17px] font-black uppercase tracking-[-0.02em] text-ink-50 truncate">{{ title }}</h3>
            <button class="font-display text-[10px] uppercase tracking-[0.14em] text-ink-400 hover:text-accent transition" @click="emit('close')">关闭</button>
          </header>
          <div class="flex-1 overflow-y-auto px-5 py-4 min-h-0">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="px-5 py-3 min-h-14 flex flex-wrap items-center justify-end gap-2 hairline-t bg-black/20">
            <slot name="footer" />
          </footer>
        </aside>
      </div>
    </transition>
  </teleport>
</template>

<style scoped>
.drawer-enter-active, .drawer-leave-active { transition: opacity .28s cubic-bezier(.25,.1,.25,1); }
.drawer-enter-active aside, .drawer-leave-active aside { transition: transform .34s cubic-bezier(.25,.1,.25,1); }
.drawer-enter-from, .drawer-leave-to { opacity: 0; }
.drawer-enter-from aside, .drawer-leave-to aside { transform: translateX(32px); }
</style>
