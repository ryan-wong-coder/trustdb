import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, IdentityView } from '@/lib/api'

export const useIdentity = defineStore('identity', () => {
  const identity = ref<IdentityView | null>(null)
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      identity.value = (await api.getIdentity()) ?? null
    } finally {
      loading.value = false
    }
  }

  async function generate(tenant: string, client: string, keyID: string) {
    identity.value = await api.generateIdentity(tenant, client, keyID)
  }

  async function rotate(newKeyID: string) {
    identity.value = await api.rotateIdentity(newKeyID)
  }

  async function importKey(tenant: string, client: string, keyID: string, priv: string) {
    identity.value = await api.importIdentity(tenant, client, keyID, priv)
  }

  async function clear() {
    await api.clearIdentity()
    identity.value = null
  }

  return { identity, loading, load, generate, rotate, importKey, clear }
})
