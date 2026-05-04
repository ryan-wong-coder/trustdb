import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { useAuth } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/dashboard' },
  { path: '/login', name: 'login', component: () => import('./pages/Login.vue'), meta: { title: '登录', public: true } },
  { path: '/dashboard', name: 'dashboard', component: () => import('./pages/Dashboard.vue'), meta: { title: '概览' } },
  { path: '/metrics', name: 'metrics', component: () => import('./pages/Metrics.vue'), meta: { title: '指标' } },
  { path: '/records', name: 'records', component: () => import('./pages/Records.vue'), meta: { title: '记录' } },
  { path: '/settings', name: 'settings', component: () => import('./pages/Settings.vue'), meta: { title: '系统设置' } },
]

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

router.beforeEach(async (to) => {
  const auth = useAuth()
  if (to.meta.public) {
    await auth.refresh()
    if (auth.ok && to.name === 'login') {
      return { path: '/dashboard' }
    }
    return true
  }
  if (!auth.loading && !auth.ok) {
    await auth.refresh()
  }
  if (!auth.ok) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  return true
})
