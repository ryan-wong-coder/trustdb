export const adminApiPrefix = '/admin/api'

export type Metric = {
  name: string
  type?: string
  help?: string
  labels?: Record<string, string>
  value: number
}

async function parseJSON<T>(res: Response): Promise<T> {
  const text = await res.text()
  try {
    return JSON.parse(text) as T
  } catch {
    throw new Error(text || res.statusText)
  }
}

export async function adminFetch(path: string, init?: RequestInit): Promise<Response> {
  const headers: Record<string, string> = {
    ...(init?.headers as Record<string, string> | undefined),
  }
  if (init?.body && typeof init.body === 'string' && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json'
  }
  return fetch(adminApiPrefix + path, {
    credentials: 'include',
    ...init,
    headers,
  })
}

export async function getSession(): Promise<{ ok: boolean; username?: string }> {
  const res = await adminFetch('/session', { method: 'GET' })
  return parseJSON(res)
}

export async function login(username: string, password: string): Promise<void> {
  const res = await adminFetch('/session', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
  const j = await parseJSON<{ ok?: boolean; error?: string }>(res)
  if (!res.ok || !j.ok) throw new Error(j.error || '登录失败')
}

export async function logout(): Promise<void> {
  await adminFetch('/session', { method: 'DELETE' })
}

export async function getMetrics(): Promise<Metric[]> {
  const res = await adminFetch('/metrics', { method: 'GET' })
  const j = await parseJSON<{ ok: boolean; metrics?: Metric[]; error?: string }>(res)
  if (!res.ok || !j.ok) throw new Error(j.error || '无法读取指标')
  return j.metrics ?? []
}

export async function getConfig(): Promise<{ config: unknown; config_path: string; notes?: string[] }> {
  const res = await adminFetch('/config', { method: 'GET' })
  const j = await parseJSON<{ ok: boolean; config?: unknown; config_path?: string; notes?: string[]; error?: string }>(res)
  if (!res.ok || !j.ok) throw new Error(j.error || '无法读取配置')
  return { config: j.config, config_path: j.config_path ?? '', notes: j.notes }
}

export async function getOverlays(): Promise<Record<string, unknown>> {
  const res = await adminFetch('/overlays', { method: 'GET' })
  const j = await parseJSON<{ ok: boolean; overlays?: Record<string, unknown>; error?: string }>(res)
  if (!res.ok || !j.ok) throw new Error(j.error || '无法读取扩展字段')
  return j.overlays ?? {}
}

export async function getConfigRaw(): Promise<string> {
  const res = await adminFetch('/config/raw', { method: 'GET' })
  if (!res.ok) {
    const t = await res.text()
    throw new Error(t || '无法读取原始配置')
  }
  return res.text()
}

export async function putConfigYaml(yaml: string): Promise<{ backup?: string }> {
  const res = await fetch(adminApiPrefix + '/config', {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/x-yaml' },
    body: yaml,
  })
  const j = await parseJSON<{ ok: boolean; backup?: string; error?: string }>(res)
  if (!res.ok || !j.ok) throw new Error(j.error || '保存失败')
  return { backup: j.backup }
}

/** GET public API through authenticated admin proxy (read-only). */
export async function proxyGet(path: string): Promise<Response> {
  const p = path.startsWith('/') ? path : `/${path}`
  return fetch(adminApiPrefix + '/proxy' + p, { credentials: 'include' })
}
