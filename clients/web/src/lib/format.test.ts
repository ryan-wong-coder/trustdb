import { describe, expect, it, vi } from 'vitest'
import { bytesToHex, copyToClipboard, nanoToDate, relativeTime, shortHash } from './format'

describe('format helpers', () => {
  it('normalizes byte-like values to hex', () => {
    expect(bytesToHex([0, 15, 255])).toBe('000fff')
    expect(bytesToHex(new Uint8Array([16, 32]))).toBe('1020')
    expect(bytesToHex('AQID')).toBe('010203')
  })

  it('shortens long hashes without touching short values', () => {
    expect(shortHash('abcdef', 3, 2)).toBe('abcdef')
    expect(shortHash('abcdefghijklmnopqrstuvwxyz', 4, 4)).toBe('abcd…wxyz')
  })

  it('renders nano timestamps as dates and relative labels', () => {
    const date = nanoToDate(1_700_000_000_000_000_000)
    expect(date?.toISOString()).toBe('2023-11-14T22:13:20.000Z')

    vi.useFakeTimers()
    vi.setSystemTime(new Date('2024-01-01T00:00:00Z'))
    expect(relativeTime(new Date('2023-12-31T23:59:00Z'))).toBe('1 分钟前')
    vi.useRealTimers()
  })

  it('falls back to textarea copy when clipboard API is unavailable', async () => {
    const originalClipboard = navigator.clipboard
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    const exec = vi.fn(() => true)
    Object.defineProperty(document, 'execCommand', { configurable: true, value: exec })

    await copyToClipboard('record-id')

    expect(exec).toHaveBeenCalledWith('copy')
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: originalClipboard })
  })
})
