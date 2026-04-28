// Typed decoder for STHAnchorResult.Proof payloads. The backend sends
// Proof as a raw byte array (Go []byte -> number[]), so the shape of
// what lives inside depends on sink_name. We only parse sinks we
// know how to render; unknown sinks fall back to the existing
// "opaque N-byte proof" display.
//
// Schema contracts (keep in sync with internal/anchor/ots_sink.go):
//
//   OtsAnchorProof (schema_version = "trustdb.anchor-ots-proof.v1")
//     calendars[] = { url, accepted, raw_timestamp?, status_code?, error?, elapsed_ms? }
//
// Bytes inside `raw_timestamp` are opaque OTS pending timestamps —
// the UI shows length + base64 head, but the full payload still
// travels in the exported .tdanchor-result for `ots upgrade`.

import { bytesToUtf8, type BytesLike } from './format'

export const OtsAnchorProofSchema = 'trustdb.anchor-ots-proof.v1'

export interface OtsCalendarTimestamp {
  url: string
  accepted: boolean
  raw_timestamp?: number[]
  status_code?: number
  error?: string
  elapsed_ms?: number
}

export interface OtsAnchorProof {
  schema_version: string
  tree_size: number
  hash_alg: string
  digest: number[]
  calendars: OtsCalendarTimestamp[]
  submitted_at_unix_nano: number
}

// decodeOtsProof returns null when the bytes don't look like an OTS
// proof envelope. Callers should fall back to a generic "N-byte
// proof" display in that case — this keeps unknown sink formats
// (future RFC 3161 TSA, custom notary, ...) from silently rendering
// as garbage.
export function decodeOtsProof(bytes: BytesLike): OtsAnchorProof | null {
  if (!bytes) return null
  const text = bytesToUtf8(bytes)
  if (!text) return null
  let parsed: any
  try { parsed = JSON.parse(text) } catch { return null }
  if (!parsed || typeof parsed !== 'object') return null
  if (parsed.schema_version !== OtsAnchorProofSchema) return null
  if (!Array.isArray(parsed.calendars)) return null
  return parsed as OtsAnchorProof
}

export function otsAcceptedCount(p: OtsAnchorProof): number {
  return p.calendars.reduce((n, c) => n + (c.accepted ? 1 : 0), 0)
}
