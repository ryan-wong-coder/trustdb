import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ToastKind = 'info' | 'success' | 'error'
export interface Toast {
  id: number
  kind: ToastKind
  title: string
  detail?: string
}

let nextID = 1

export const useToasts = defineStore('toasts', () => {
  const items = ref<Toast[]>([])

  function push(kind: ToastKind, title: string, detail?: string) {
    const id = nextID++
    items.value.push({ id, kind, title, detail })
    // Auto-dismiss after a short window; errors linger a bit longer
    // so the user has time to read a stack trace before it goes away.
    setTimeout(() => dismiss(id), kind === 'error' ? 6500 : 3500)
  }

  function dismiss(id: number) {
    items.value = items.value.filter((t) => t.id !== id)
  }

  function info(title: string, detail?: string)    { push('info', title, detail) }
  function success(title: string, detail?: string) { push('success', title, detail) }
  function error(title: string, detail?: string)   { push('error', title, detail) }

  return { items, push, dismiss, info, success, error }
})
