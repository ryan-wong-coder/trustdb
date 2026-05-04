import { defineStore } from 'pinia'
import { ref } from 'vue'
import { getSession, login as apiLogin, logout as apiLogout } from '@/lib/api'

export const useAuth = defineStore('auth', () => {
  const ok = ref(false)
  const username = ref<string | null>(null)
  const loading = ref(false)

  async function refresh() {
    loading.value = true
    try {
      const s = await getSession()
      ok.value = !!s.ok
      username.value = s.username ?? null
    } catch {
      ok.value = false
      username.value = null
    } finally {
      loading.value = false
    }
  }

  async function login(user: string, password: string) {
    await apiLogin(user, password)
    await refresh()
  }

  async function logout() {
    await apiLogout()
    ok.value = false
    username.value = null
  }

  return { ok, username, loading, refresh, login, logout }
})
