import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getMetrics, login, proxyGet, putConfigYaml } from './api'

const fetchMock = vi.fn()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('admin API facade', () => {
  it('logs in with JSON body and credentials included', async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), { status: 200 }))

    await login('root', 'secret')

    expect(fetchMock).toHaveBeenCalledWith('/admin/api/session', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      body: JSON.stringify({ username: 'root', password: 'secret' }),
    }))
  })

  it('raises server login errors', async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ ok: false, error: 'unauthorized' }), { status: 401 }))

    await expect(login('root', 'bad')).rejects.toThrow('unauthorized')
  })

  it('loads metrics from the protected JSON endpoint', async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      ok: true,
      metrics: [{ name: 'trustdb_batch_pending', value: 3 }],
    }), { status: 200 }))

    await expect(getMetrics()).resolves.toEqual([{ name: 'trustdb_batch_pending', value: 3 }])
  })

  it('writes YAML through PUT /config', async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ ok: true, backup: 'trustdb.yaml.bak' }), { status: 200 }))

    await expect(putConfigYaml('admin:\n  enabled: true\n')).resolves.toEqual({ backup: 'trustdb.yaml.bak' })
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/config', expect.objectContaining({
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/x-yaml' },
    }))
  })

  it('builds read-only proxy URLs under /admin/api/proxy', async () => {
    fetchMock.mockResolvedValueOnce(new Response('{}', { status: 200 }))

    await proxyGet('/v1/records?limit=5')

    expect(fetchMock).toHaveBeenCalledWith('/admin/api/proxy/v1/records?limit=5', { credentials: 'include' })
  })
})
