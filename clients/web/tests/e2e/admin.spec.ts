import { expect, test, type Page } from '@playwright/test'

async function mockAdminAPI(page: Page) {
  let loggedIn = false

  await page.route('**/admin/api/session', async (route) => {
    const method = route.request().method()
    if (method === 'GET') {
      await route.fulfill({ json: { ok: loggedIn, username: loggedIn ? 'ops' : undefined } })
      return
    }
    if (method === 'POST') {
      const body = route.request().postDataJSON() as { username?: string; password?: string }
      loggedIn = body.username === 'ops' && body.password === 'secret'
      await route.fulfill({ status: loggedIn ? 200 : 401, json: loggedIn ? { ok: true } : { ok: false, error: 'unauthorized' } })
      return
    }
    if (method === 'DELETE') {
      loggedIn = false
      await route.fulfill({ json: { ok: true } })
      return
    }
    await route.fulfill({ status: 405, body: 'method not allowed' })
  })

  await page.route('**/admin/api/overlays', async (route) => {
    await route.fulfill({
      json: {
        ok: true,
        overlays: {
          server: { grpc_listen: '127.0.0.1:9090' },
          metastore: 'pebble',
          anchor: { sink: 'ots' },
        },
      },
    })
  })

  await page.route('**/admin/api/metrics', async (route) => {
    await route.fulfill({
      json: {
        ok: true,
        metrics: [
          { name: 'trustdb_ingest_accepted_total', type: 'counter', value: 12 },
          { name: 'trustdb_batch_pending', type: 'gauge', value: 3 },
          { name: 'trustdb_anchor_pending', type: 'gauge', value: 1 },
          { name: 'trustdb_custom_latency_seconds', type: 'gauge', labels: { quantile: '0.95' }, value: 0.42 },
        ],
      },
    })
  })

  await page.route('**/admin/api/config', async (route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        json: {
          ok: true,
          config_path: 'C:/trustdb/trustdb.yaml',
          config: {
            admin: { enabled: true, username: 'ops', password_hash: '<redacted>', session_secret: '<redacted>' },
            server: { listen: '127.0.0.1:8080' },
          },
        },
      })
      return
    }
    if (route.request().method() === 'PUT') {
      await route.fulfill({ json: { ok: true, backup: 'C:/trustdb/trustdb.yaml.bak.1' } })
      return
    }
    await route.fulfill({ status: 405, body: 'method not allowed' })
  })

  await page.route('**/admin/api/config/raw', async (route) => {
    await route.fulfill({
      contentType: 'application/x-yaml',
      body: 'admin:\n  enabled: true\nserver:\n  listen: "127.0.0.1:8080"\n',
    })
  })

  await page.route('**/admin/api/proxy/healthz', async (route) => {
    await route.fulfill({ json: { ok: true } })
  })

  await page.route('**/admin/api/proxy/v1/roots/latest', async (route) => {
    await route.fulfill({ json: { batch_id: 'batch-1', tree_size: 2 } })
  })

  await page.route('**/admin/api/proxy/v1/records?*', async (route) => {
    await route.fulfill({
      json: {
        records: [
          {
            record_id: 'record-abcdef123456',
            proof_level: 'L5',
            tenant_id: 'tenant-a',
            client_id: 'client-a',
            batch_id: 'batch-1',
            received_at_unix_n: 1_700_000_000_000_000_000,
          },
        ],
        limit: 50,
        direction: 'desc',
      },
    })
  })
}

test.beforeEach(async ({ page }) => {
  await mockAdminAPI(page)
})

test('requires login and opens the admin dashboard', async ({ page }) => {
  await page.goto('/admin/dashboard')

  await expect(page).toHaveURL(/\/admin\/login/)
  await page.locator('input').nth(0).fill('ops')
  await page.locator('input').nth(1).fill('secret')
  await page.getByRole('button', { name: '登录' }).click()

  await expect(page).toHaveURL(/\/admin\/dashboard/)
  await expect(page.getByRole('heading', { name: '运维概览' })).toBeVisible()
  await expect(page.getByText('pebble')).toBeVisible()
  await expect(page.getByText('batch-1')).toBeVisible()
})

test('shows metrics and records through authenticated admin APIs', async ({ page }) => {
  await page.goto('/admin/login')
  await page.locator('input').nth(0).fill('ops')
  await page.locator('input').nth(1).fill('secret')
  await page.getByRole('button', { name: '登录' }).click()

  await page.getByRole('link', { name: /指标/ }).click()
  await expect(page.getByText('claims accepted')).toBeVisible()
  await expect(page.getByText('12', { exact: true })).toBeVisible()
  await expect(page.getByText('trustdb_custom_latency_seconds')).toBeVisible()

  await page.getByRole('link', { name: /记录/ }).click()
  await expect(page.getByText('tenant-a')).toBeVisible()
  await expect(page.getByText('client-a')).toBeVisible()
  await expect(page.getByText('batch-1')).toBeVisible()
})

test('loads settings and saves YAML through the config endpoint', async ({ page }) => {
  await page.goto('/admin/login')
  await page.locator('input').nth(0).fill('ops')
  await page.locator('input').nth(1).fill('secret')
  await page.getByRole('button', { name: '登录' }).click()

  await page.getByRole('link', { name: /系统设置/ }).click()
  await expect(page.getByText('C:/trustdb/trustdb.yaml')).toBeVisible()
  await expect(page.locator('textarea')).toHaveValue(/admin:/)
  await page.getByRole('button', { name: '保存 YAML' }).click()
  await expect(page.getByText(/已保存/)).toBeVisible()
})
