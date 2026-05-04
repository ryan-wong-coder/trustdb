import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getMetrics } from '@/lib/api'
import Metrics from './Metrics.vue'

vi.mock('@/lib/api', () => ({
  getMetrics: vi.fn(),
}))

const mockedGetMetrics = vi.mocked(getMetrics)

describe('Metrics page', () => {
  beforeEach(() => {
    mockedGetMetrics.mockReset()
  })

  it('renders highlighted metric cards and raw grouped metrics', async () => {
    mockedGetMetrics.mockResolvedValueOnce([
      { name: 'trustdb_ingest_accepted_total', value: 7, type: 'counter' },
      { name: 'trustdb_batch_pending', value: 2, type: 'gauge' },
      { name: 'trustdb_custom_latency_seconds', value: 0.42, type: 'gauge', labels: { quantile: '0.95' } },
    ])

    const wrapper = mount(Metrics)
    await flushPromises()

    expect(wrapper.text()).toContain('claims accepted')
    expect(wrapper.text()).toContain('7')
    expect(wrapper.text()).toContain('batch pending')
    expect(wrapper.text()).toContain('trustdb_custom_latency_seconds')
    expect(wrapper.text()).toContain('quantile="0.95"')
    wrapper.unmount()
  })

  it('surfaces load errors in the page status', async () => {
    mockedGetMetrics.mockRejectedValueOnce(new Error('metrics unavailable'))

    const wrapper = mount(Metrics)
    await flushPromises()

    expect(wrapper.text()).toContain('metrics unavailable')
    expect(wrapper.text()).toContain('暂无指标数据')
    wrapper.unmount()
  })
})
