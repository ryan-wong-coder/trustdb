package anchor

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/ripemd160"
	"golang.org/x/crypto/sha3"
)

// OTS binary timestamp format.
//
// Reference implementations:
//   - https://github.com/opentimestamps/python-opentimestamps
//     (binary.py / timestamp.py / ops.py / attestation.py)
//   - https://github.com/opentimestamps/javascript-opentimestamps
//
// A serialized timestamp is a tree of operations. Each "next item"
// off a node is either an op (with its own child subtree) or an
// attestation (a leaf). Sibling items are written back-to-back; if
// there is a sibling AFTER the current item it is preceded by 0xff.
// The LAST sibling at any node is NOT preceded by 0xff. The parser
// loop therefore consumes one item, then peeks for 0xff to decide
// whether another sibling follows.
//
// Op tags:
//
//	0x02 sha1            sha1(msg)
//	0x03 ripemd160       ripemd160(msg)
//	0x08 sha256          sha256(msg)
//	0x67 keccak256       keccak256(msg)
//	0xf0 append          msg || varbytes
//	0xf1 prepend         varbytes || msg
//
// Attestation magics (8 bytes, fixed):
//
//	83 df e3 0d 2e f9 0c 8e   PendingAttestation        (payload: varbytes URI)
//	05 88 96 0d 73 d7 19 01   BitcoinBlockHeaderAttest  (payload: varuint height)
//	06 86 9a 0d 73 d7 1b 45   LitecoinBlockHeaderAttest (payload: varuint height)
//	30 fe 80 87 b5 c7 ea d7   EthereumBlockHeaderAttest (payload: varuint height)
//
// For the BitcoinBlockHeaderAttestation case, the "current message"
// at the leaf — i.e. the digest derived by walking the path of ops
// from the original SHA-256 of the user's data — is the candidate
// merkle root of block #height. Verification is then "fetch the
// real block header for #height from a trusted source and compare
// merkle_root fields byte-for-byte". That comparison is the job of
// internal/verify; this file only extracts what the proof claims.

// OtsAttestationKind tags the four attestation types we recognise
// plus a catch-all for forward compatibility.
type OtsAttestationKind string

const (
	OtsAttestPending  OtsAttestationKind = "pending"
	OtsAttestBitcoin  OtsAttestationKind = "bitcoin"
	OtsAttestLitecoin OtsAttestationKind = "litecoin"
	OtsAttestEthereum OtsAttestationKind = "ethereum"
	OtsAttestUnknown  OtsAttestationKind = "unknown"
)

// Magic bytes for each attestation kind. Keep them as hex strings so
// the switch in parseAttestation reads as a flat lookup table; the
// few extra allocs per leaf are negligible compared to the SHA-256
// work the parser already does.
const (
	otsMagicPending  = "83dfe30d2ef90c8e"
	otsMagicBitcoin  = "0588960d73d71901"
	otsMagicLitecoin = "06869a0d73d71b45"
	otsMagicEthereum = "30fe8087b5c7ead7"
)

// OtsAttestation is one leaf attestation extracted from a parsed
// OTS timestamp tree, paired with the digest derived along the
// path that reached it.
type OtsAttestation struct {
	Kind        OtsAttestationKind `json:"kind"`
	BlockHeight uint64             `json:"block_height,omitempty"`
	PendingURI  string             `json:"pending_uri,omitempty"`
	// MerkleRoot is the message bytes at the attestation leaf. For
	// a Bitcoin attestation this is the candidate merkle root that
	// must equal block(BlockHeight).merkle_root. For Pending it is
	// the digest the calendar promised to commit. Always 20 / 32
	// bytes depending on the path.
	MerkleRoot []byte `json:"merkle_root"`
	// UnknownTag carries the raw 8-byte magic when Kind is Unknown
	// so callers can log/forward without losing information.
	UnknownTag []byte `json:"unknown_tag,omitempty"`
}

// OtsParsedTimestamp is the flattened set of attestations a single
// OTS proof leads to. Order is the depth-first traversal order of
// the on-disk tree, which keeps tests deterministic without anyone
// having to sort by magic-or-height.
type OtsParsedTimestamp struct {
	Attestations []OtsAttestation `json:"attestations"`
}

// BitcoinAttestations returns just the BitcoinBlockHeaderAttestation
// leaves. Most callers (UI badge, verify path) only care about these.
func (p *OtsParsedTimestamp) BitcoinAttestations() []OtsAttestation {
	if p == nil {
		return nil
	}
	out := make([]OtsAttestation, 0, len(p.Attestations))
	for _, a := range p.Attestations {
		if a.Kind == OtsAttestBitcoin {
			out = append(out, a)
		}
	}
	return out
}

// HasBitcoin reports whether at least one Bitcoin block-header
// attestation was found. Equivalent to "the calendar has been
// upgraded past the pending state for this digest".
func (p *OtsParsedTimestamp) HasBitcoin() bool {
	return len(p.BitcoinAttestations()) > 0
}

// HasPending reports whether any branch is still in the pending
// state. After a successful upgrade Bitcoin and Pending leaves can
// coexist (some calendars upgraded, others still waiting).
func (p *OtsParsedTimestamp) HasPending() bool {
	if p == nil {
		return false
	}
	for _, a := range p.Attestations {
		if a.Kind == OtsAttestPending {
			return true
		}
	}
	return false
}

// Hard limits guard against pathological input. OTS proofs in the
// wild are tiny (<1 KB pending, <1 KB upgraded). The numbers below
// give us 6+ orders of magnitude of headroom while still rejecting
// adversarial blobs that try to make us allocate gigabytes.
const (
	otsMaxVarbytesLen        = 1 << 20 // 1 MiB
	otsMaxAttestPayloadLen   = 1 << 16 // 64 KiB
	otsMaxRecursionDepth     = 1024    // path depth, not breadth
	otsMaxAttestationsPerProof = 1024  // breadth — guards against DoS via 0xff spam
)

// ParseOtsTimestamp walks the binary tree starting from initialDigest
// (typically the SHA-256 of the original user data) and returns every
// attestation leaf together with the digest that reaches it.
//
// Errors are returned only for truly malformed input (truncated
// stream, unsupported op tag, depth/breadth limits exceeded). An
// unknown attestation magic is not an error: the leaf is recorded
// with Kind=Unknown so callers can decide how strict to be.
func ParseOtsTimestamp(initialDigest, raw []byte) (*OtsParsedTimestamp, error) {
	if len(initialDigest) == 0 {
		return nil, errors.New("ots: empty initial digest")
	}
	if len(raw) == 0 {
		return nil, errors.New("ots: empty timestamp body")
	}
	p := &otsParser{
		r:   bytes.NewReader(raw),
		out: &OtsParsedTimestamp{},
	}
	if err := p.walk(append([]byte(nil), initialDigest...), 0); err != nil {
		return nil, err
	}
	if p.r.Len() != 0 {
		return nil, fmt.Errorf("ots: %d trailing bytes after timestamp", p.r.Len())
	}
	return p.out, nil
}

type otsParser struct {
	r   *bytes.Reader
	out *OtsParsedTimestamp
}

func (p *otsParser) walk(currentMsg []byte, depth int) error {
	if depth > otsMaxRecursionDepth {
		return fmt.Errorf("ots: tree depth %d exceeds limit %d", depth, otsMaxRecursionDepth)
	}
	for {
		tag, err := p.r.ReadByte()
		if err != nil {
			return fmt.Errorf("ots: read item tag: %w", err)
		}
		more := false
		if tag == 0xff {
			more = true
			tag, err = p.r.ReadByte()
			if err != nil {
				return fmt.Errorf("ots: read sibling tag after 0xff: %w", err)
			}
		}
		if err := p.parseItem(tag, currentMsg, depth); err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

func (p *otsParser) parseItem(tag byte, currentMsg []byte, depth int) error {
	if tag == 0x00 {
		return p.parseAttestation(currentMsg)
	}
	next, err := p.applyOp(tag, currentMsg)
	if err != nil {
		return err
	}
	return p.walk(next, depth+1)
}

func (p *otsParser) applyOp(tag byte, msg []byte) ([]byte, error) {
	switch tag {
	case 0x08: // sha256
		h := sha256.Sum256(msg)
		return h[:], nil
	case 0x02: // sha1
		h := sha1.Sum(msg)
		return h[:], nil
	case 0x03: // ripemd160
		h := ripemd160.New()
		_, _ = h.Write(msg)
		return h.Sum(nil), nil
	case 0x67: // keccak256
		h := sha3.NewLegacyKeccak256()
		_, _ = h.Write(msg)
		return h.Sum(nil), nil
	case 0xf0: // append
		data, err := p.readVarbytes()
		if err != nil {
			return nil, fmt.Errorf("ots: append payload: %w", err)
		}
		out := make([]byte, 0, len(msg)+len(data))
		out = append(out, msg...)
		out = append(out, data...)
		return out, nil
	case 0xf1: // prepend
		data, err := p.readVarbytes()
		if err != nil {
			return nil, fmt.Errorf("ots: prepend payload: %w", err)
		}
		out := make([]byte, 0, len(msg)+len(data))
		out = append(out, data...)
		out = append(out, msg...)
		return out, nil
	default:
		return nil, fmt.Errorf("ots: unsupported op tag 0x%02x", tag)
	}
}

func (p *otsParser) parseAttestation(currentMsg []byte) error {
	if len(p.out.Attestations) >= otsMaxAttestationsPerProof {
		return fmt.Errorf("ots: attestation count exceeds limit %d", otsMaxAttestationsPerProof)
	}
	var magic [8]byte
	if _, err := io.ReadFull(p.r, magic[:]); err != nil {
		return fmt.Errorf("ots: attestation magic: %w", err)
	}
	payloadLen, err := p.readVaruint()
	if err != nil {
		return fmt.Errorf("ots: attestation length: %w", err)
	}
	if payloadLen > otsMaxAttestPayloadLen {
		return fmt.Errorf("ots: attestation payload %d > %d", payloadLen, otsMaxAttestPayloadLen)
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(p.r, payload); err != nil {
		return fmt.Errorf("ots: attestation payload: %w", err)
	}
	att := OtsAttestation{
		MerkleRoot: append([]byte(nil), currentMsg...),
	}
	switch hex.EncodeToString(magic[:]) {
	case otsMagicBitcoin:
		att.Kind = OtsAttestBitcoin
		h, _, err := parseAttestationVaruint(payload)
		if err != nil {
			return fmt.Errorf("ots: bitcoin block height: %w", err)
		}
		att.BlockHeight = h
	case otsMagicLitecoin:
		att.Kind = OtsAttestLitecoin
		h, _, err := parseAttestationVaruint(payload)
		if err != nil {
			return fmt.Errorf("ots: litecoin block height: %w", err)
		}
		att.BlockHeight = h
	case otsMagicEthereum:
		att.Kind = OtsAttestEthereum
		h, _, err := parseAttestationVaruint(payload)
		if err != nil {
			return fmt.Errorf("ots: ethereum block height: %w", err)
		}
		att.BlockHeight = h
	case otsMagicPending:
		att.Kind = OtsAttestPending
		uri, _, err := parseAttestationVarbytes(payload)
		if err != nil {
			return fmt.Errorf("ots: pending uri: %w", err)
		}
		att.PendingURI = uri
	default:
		att.Kind = OtsAttestUnknown
		att.UnknownTag = append([]byte(nil), magic[:]...)
	}
	p.out.Attestations = append(p.out.Attestations, att)
	return nil
}

func (p *otsParser) readVaruint() (uint64, error) {
	var v uint64
	var shift uint
	for shift < 64 {
		b, err := p.r.ReadByte()
		if err != nil {
			return 0, err
		}
		v |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return v, nil
		}
		shift += 7
	}
	return 0, errors.New("ots: varuint overflow (>64 bits)")
}

func (p *otsParser) readVarbytes() ([]byte, error) {
	n, err := p.readVaruint()
	if err != nil {
		return nil, err
	}
	if n > otsMaxVarbytesLen {
		return nil, fmt.Errorf("ots: varbytes length %d > %d", n, otsMaxVarbytesLen)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(p.r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// parseAttestationVaruint reads a varuint from a payload buffer
// (rather than the streaming reader) and returns the consumed
// length. Used to decode attestation-level payloads, which the
// outer parser hands us as a complete byte slice.
func parseAttestationVaruint(payload []byte) (uint64, int, error) {
	var v uint64
	var shift uint
	for i, b := range payload {
		v |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return v, i + 1, nil
		}
		shift += 7
		if shift >= 64 {
			return 0, 0, errors.New("ots: payload varuint overflow (>64 bits)")
		}
	}
	return 0, 0, errors.New("ots: payload varuint truncated")
}

func parseAttestationVarbytes(payload []byte) (string, int, error) {
	n, off, err := parseAttestationVaruint(payload)
	if err != nil {
		return "", 0, err
	}
	if int(n) > len(payload)-off {
		return "", 0, fmt.Errorf("ots: payload varbytes %d > remaining %d", n, len(payload)-off)
	}
	return string(payload[off : off+int(n)]), off + int(n), nil
}
