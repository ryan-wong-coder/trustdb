// BytesLike covers every shape a Go []byte can arrive in on the JS
// side. The Wails-generated TypeScript types annotate Go []byte as
// `number[]`, but at runtime Go's encoding/json marshals []byte as a
// standard-base64 *string*. Both forms (and the legacy Uint8Array)
// must round-trip through the helpers below, so callers don't need
// to remember which one they're holding.
export type BytesLike = number[] | Uint8Array | string | undefined | null

// coerceBytes normalises every supported wire shape to Uint8Array.
// Input contracts:
//   - null / undefined / "" → empty Uint8Array (callers can render "—")
//   - Uint8Array            → returned unchanged
//   - number[]              → wrapped via Uint8Array.from (no copy of values)
//   - string                → decoded as standard base64; malformed
//                             input collapses to empty so a single bad
//                             field can't tank the whole render.
//
// Centralising the type guard here means bytesToHex / bytesToUtf8 /
// bytesToBase64 / decodeOtsProof all share one codepath instead of
// duplicating the (subtly wrong) `instanceof Uint8Array ? ... : new
// Uint8Array(x)` pattern that silently turned base64 strings into
// empty arrays.
export function coerceBytes(bytes: BytesLike): Uint8Array {
  if (!bytes) return new Uint8Array(0)
  if (bytes instanceof Uint8Array) return bytes
  if (typeof bytes === 'string') {
    try {
      const bin = atob(bytes)
      const out = new Uint8Array(bin.length)
      for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
      return out
    } catch {
      return new Uint8Array(0)
    }
  }
  return Uint8Array.from(bytes)
}

export function bytesToHex(bytes: BytesLike): string {
  const arr = coerceBytes(bytes)
  if (arr.length === 0) return ''
  let s = ''
  for (let i = 0; i < arr.length; i++) {
    s += arr[i].toString(16).padStart(2, '0')
  }
  return s
}

// bytesToUtf8 decodes a byte array coming back from Wails. Used for
// JSON envelopes like the OpenTimestamps anchor proof, where the
// bytes are actually textual JSON rather than an opaque binary blob.
export function bytesToUtf8(bytes: BytesLike): string {
  const arr = coerceBytes(bytes)
  if (arr.length === 0) return ''
  try {
    return new TextDecoder('utf-8', { fatal: false }).decode(arr)
  } catch {
    return ''
  }
}

// bytesToBase64 produces a compact, human-presentable rendering of
// opaque bytes (e.g. OTS raw_timestamp pending proofs). When the
// caller already holds a base64 string we round-trip via coerceBytes
// to canonicalise padding/whitespace; the cost is one decode + one
// encode, which is fine for the small payloads this helper sees.
export function bytesToBase64(bytes: BytesLike): string {
  const arr = coerceBytes(bytes)
  if (arr.length === 0) return ''
  let bin = ''
  for (let i = 0; i < arr.length; i++) bin += String.fromCharCode(arr[i])
  try { return btoa(bin) } catch { return '' }
}

export function shortHash(hex: string, head = 8, tail = 6): string {
  if (!hex) return ''
  if (hex.length <= head + tail + 1) return hex
  return `${hex.slice(0, head)}…${hex.slice(-tail)}`
}

export function shortID(id: string, head = 10, tail = 6): string {
  return shortHash(id, head, tail)
}

export function humanSize(n: number | undefined): string {
  if (n == null || !Number.isFinite(n)) return '—'
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

export function relativeTime(input: string | number | Date | undefined): string {
  if (!input) return '—'
  const t = typeof input === 'number' ? new Date(input) : new Date(input)
  const ms = Date.now() - t.getTime()
  if (!Number.isFinite(ms)) return '—'
  if (ms < 5000) return '刚刚'
  if (ms < 60_000) return `${Math.round(ms / 1000)} 秒前`
  if (ms < 3_600_000) return `${Math.round(ms / 60_000)} 分钟前`
  if (ms < 86_400_000) return `${Math.round(ms / 3_600_000)} 小时前`
  return `${Math.round(ms / 86_400_000)} 天前`
}

export function formatTime(input: string | number | Date | undefined): string {
  if (!input) return ''
  const t = new Date(input)
  if (Number.isNaN(t.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())} ${pad(t.getHours())}:${pad(t.getMinutes())}:${pad(t.getSeconds())}`
}

export function nanoToDate(ns: number | undefined): Date | undefined {
  if (ns == null || ns <= 0) return undefined
  return new Date(ns / 1_000_000)
}

export function copyToClipboard(text: string): Promise<void> {
  if (navigator.clipboard && window.isSecureContext) {
    return navigator.clipboard.writeText(text)
  }
  return new Promise<void>((resolve) => {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    document.body.removeChild(ta)
    resolve()
  })
}
