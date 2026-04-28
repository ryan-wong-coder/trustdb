<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { CheckCircle2, AlertTriangle, Info, X } from 'lucide-vue-next'
import { useToasts } from '@/stores/toasts'

const toasts = useToasts()
const { items } = storeToRefs(toasts)

function icon(kind: string) {
  if (kind === 'success') return CheckCircle2
  if (kind === 'error') return AlertTriangle
  return Info
}
</script>

<template>
  <teleport to="body">
    <div class="fixed top-4 right-4 z-50 flex flex-col gap-2 pointer-events-none max-w-[380px]">
      <transition-group name="toast">
        <div
          v-for="t in items"
          :key="t.id"
          class="pointer-events-auto glass rounded-[18px] px-4 py-3 flex items-start gap-3 shadow-soft border border-white/10"
        >
          <component :is="icon(t.kind)"
            :size="16"
            :class="{
              'text-success': t.kind === 'success',
              'text-danger':  t.kind === 'error',
              'text-accent':  t.kind === 'info',
            }"
            class="mt-0.5 shrink-0"
          />
          <div class="min-w-0 flex-1">
            <p class="font-display text-[14px] font-bold uppercase tracking-[0.02em] text-ink-50 truncate">{{ t.title }}</p>
            <p v-if="t.detail" class="text-[12px] text-ink-400 mt-0.5 break-words">{{ t.detail }}</p>
          </div>
          <button class="text-ink-500 hover:text-accent transition" @click="toasts.dismiss(t.id)">
            <X :size="14" />
          </button>
        </div>
      </transition-group>
    </div>
  </teleport>
</template>

<style scoped>
.toast-enter-active, .toast-leave-active { transition: all .3s cubic-bezier(.25,.1,.25,1); }
.toast-enter-from { opacity: 0; transform: translateY(-8px) scale(.98); }
.toast-leave-to   { opacity: 0; transform: translateX(20px); }
</style>
