package anchor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// otsBuilder lays out OTS binary timestamps by hand for tests.
// Production code never needs to encode OTS — the parser is one-way
// — so this helper lives here rather than alongside ots_parse.go.
type otsBuilder struct {
	buf bytes.Buffer
}

func (b *otsBuilder) writeVaruint(v uint64) {
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			c |= 0x80
		}
		b.buf.WriteByte(c)
		if v == 0 {
			return
		}
	}
}

func (b *otsBuilder) writeVarbytes(p []byte) {
	b.writeVaruint(uint64(len(p)))
	b.buf.Write(p)
}

// sibling emits the 0xff marker that says "another sibling follows
// this one". The LAST sibling at any node must NOT be preceded by it.
func (b *otsBuilder) sibling() { b.buf.WriteByte(0xff) }

func (b *otsBuilder) opSHA256()        { b.buf.WriteByte(0x08) }
func (b *otsBuilder) opSHA1()          { b.buf.WriteByte(0x02) }
func (b *otsBuilder) opRIPEMD160()     { b.buf.WriteByte(0x03) }
func (b *otsBuilder) opKECCAK256()     { b.buf.WriteByte(0x67) }
func (b *otsBuilder) opAppend(p []byte) {
	b.buf.WriteByte(0xf0)
	b.writeVarbytes(p)
}
func (b *otsBuilder) opPrepend(p []byte) {
	b.buf.WriteByte(0xf1)
	b.writeVarbytes(p)
}

// attBitcoin emits a BitcoinBlockHeaderAttestation leaf. Layout:
// 0x00 | magic(8) | varuint(payload_len) | varuint(height)
func (b *otsBuilder) attBitcoin(height uint64) {
	b.buf.WriteByte(0x00)
	mustHex(&b.buf, otsMagicBitcoin)
	var payload otsBuilder
	payload.writeVaruint(height)
	b.writeVarbytes(payload.buf.Bytes())
}

func (b *otsBuilder) attLitecoin(height uint64) {
	b.buf.WriteByte(0x00)
	mustHex(&b.buf, otsMagicLitecoin)
	var payload otsBuilder
	payload.writeVaruint(height)
	b.writeVarbytes(payload.buf.Bytes())
}

func (b *otsBuilder) attPending(uri string) {
	b.buf.WriteByte(0x00)
	mustHex(&b.buf, otsMagicPending)
	var payload otsBuilder
	payload.writeVarbytes([]byte(uri))
	b.writeVarbytes(payload.buf.Bytes())
}

// attUnknown emits an attestation with an arbitrary 8-byte magic.
// magicHex must be exactly 16 hex chars.
func (b *otsBuilder) attUnknown(magicHex string, payload []byte) {
	b.buf.WriteByte(0x00)
	mustHex(&b.buf, magicHex)
	b.writeVarbytes(payload)
}

func (b *otsBuilder) bytes() []byte { return b.buf.Bytes() }

func mustHex(w *bytes.Buffer, s string) {
	raw, err := hex.DecodeString(s)
	if err != nil {
		panic("test bug: bad hex literal: " + s)
	}
	w.Write(raw)
}

func TestParseOtsTimestamp_SingleBitcoinAttestation(t *testing.T) {
	initial := []byte{0x01, 0x02, 0x03, 0x04}
	want := sha256.Sum256(append(append([]byte(nil), initial...), 'a', 'b', 'c'))

	var b otsBuilder
	b.opAppend([]byte("abc"))
	b.opSHA256()
	b.attBitcoin(800123)

	got, err := ParseOtsTimestamp(initial, b.bytes())
	if err != nil {
		t.Fatalf("ParseOtsTimestamp: %v", err)
	}
	if len(got.Attestations) != 1 {
		t.Fatalf("attestations = %d, want 1", len(got.Attestations))
	}
	att := got.Attestations[0]
	if att.Kind != OtsAttestBitcoin {
		t.Fatalf("kind = %s, want bitcoin", att.Kind)
	}
	if att.BlockHeight != 800123 {
		t.Fatalf("height = %d, want 800123", att.BlockHeight)
	}
	if !bytes.Equal(att.MerkleRoot, want[:]) {
		t.Fatalf("merkle root mismatch:\ngot  %x\nwant %x", att.MerkleRoot, want[:])
	}
}

func TestParseOtsTimestamp_BranchPendingPlusBitcoin(t *testing.T) {
	initial := sha256.Sum256([]byte("hello"))
	const uri = "https://a.pool.opentimestamps.org"

	var b otsBuilder
	b.sibling()
	b.attPending(uri)
	// Last sibling is the bitcoin chain (no leading 0xff).
	b.opAppend([]byte{0x01, 0x02, 0x03, 0x04})
	b.opSHA256()
	b.attBitcoin(800123)

	got, err := ParseOtsTimestamp(initial[:], b.bytes())
	if err != nil {
		t.Fatalf("ParseOtsTimestamp: %v", err)
	}
	if len(got.Attestations) != 2 {
		t.Fatalf("attestations = %d, want 2", len(got.Attestations))
	}
	if got.Attestations[0].Kind != OtsAttestPending {
		t.Fatalf("first kind = %s, want pending", got.Attestations[0].Kind)
	}
	if got.Attestations[0].PendingURI != uri {
		t.Fatalf("pending uri = %q, want %q", got.Attestations[0].PendingURI, uri)
	}
	// Pending attestation's merkle_root is just the current message at
	// that point (no path applied), i.e. the original hash.
	if !bytes.Equal(got.Attestations[0].MerkleRoot, initial[:]) {
		t.Fatalf("pending merkle root mismatch:\ngot  %x\nwant %x", got.Attestations[0].MerkleRoot, initial[:])
	}
	if got.Attestations[1].Kind != OtsAttestBitcoin {
		t.Fatalf("second kind = %s, want bitcoin", got.Attestations[1].Kind)
	}
	if got.Attestations[1].BlockHeight != 800123 {
		t.Fatalf("height = %d, want 800123", got.Attestations[1].BlockHeight)
	}
	wantMerkle := sha256.Sum256(append(append([]byte(nil), initial[:]...), 0x01, 0x02, 0x03, 0x04))
	if !bytes.Equal(got.Attestations[1].MerkleRoot, wantMerkle[:]) {
		t.Fatalf("bitcoin merkle root mismatch:\ngot  %x\nwant %x", got.Attestations[1].MerkleRoot, wantMerkle[:])
	}
	if !got.HasBitcoin() {
		t.Fatal("HasBitcoin() = false")
	}
	if !got.HasPending() {
		t.Fatal("HasPending() = false")
	}
	if n := len(got.BitcoinAttestations()); n != 1 {
		t.Fatalf("BitcoinAttestations() = %d, want 1", n)
	}
}

func TestParseOtsTimestamp_PrependAndAllHashes(t *testing.T) {
	// Confirm prepend + sha1/ripemd160/keccak256 paths all reach
	// the same attestation. We don't need a deep verification of
	// the digest values; presence of a successful parse with a
	// non-empty merkle root is enough — applyOp is the unit under
	// test.
	initial := []byte{0xaa, 0xbb}

	cases := []struct {
		name string
		emit func(*otsBuilder)
	}{
		{"prepend+sha256", func(b *otsBuilder) { b.opPrepend([]byte{0x99}); b.opSHA256() }},
		{"sha1", func(b *otsBuilder) { b.opSHA1() }},
		{"ripemd160", func(b *otsBuilder) { b.opRIPEMD160() }},
		{"keccak256", func(b *otsBuilder) { b.opKECCAK256() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b otsBuilder
			tc.emit(&b)
			b.attLitecoin(2_500_000)

			got, err := ParseOtsTimestamp(initial, b.bytes())
			if err != nil {
				t.Fatalf("ParseOtsTimestamp: %v", err)
			}
			if len(got.Attestations) != 1 {
				t.Fatalf("attestations = %d, want 1", len(got.Attestations))
			}
			if got.Attestations[0].Kind != OtsAttestLitecoin {
				t.Fatalf("kind = %s", got.Attestations[0].Kind)
			}
			if len(got.Attestations[0].MerkleRoot) == 0 {
				t.Fatal("merkle root empty")
			}
		})
	}
}

func TestParseOtsTimestamp_UnknownAttestationMagicIsPreserved(t *testing.T) {
	const magic = "deadbeefdeadbeef"
	var b otsBuilder
	b.attUnknown(magic, []byte{0x42, 0x43})

	got, err := ParseOtsTimestamp([]byte{0x00}, b.bytes())
	if err != nil {
		t.Fatalf("ParseOtsTimestamp: %v", err)
	}
	if len(got.Attestations) != 1 {
		t.Fatalf("attestations = %d, want 1", len(got.Attestations))
	}
	att := got.Attestations[0]
	if att.Kind != OtsAttestUnknown {
		t.Fatalf("kind = %s, want unknown", att.Kind)
	}
	if hex.EncodeToString(att.UnknownTag) != magic {
		t.Fatalf("unknown tag = %x, want %s", att.UnknownTag, magic)
	}
}

func TestParseOtsTimestamp_RejectsTrailingBytes(t *testing.T) {
	var b otsBuilder
	b.attBitcoin(1)
	raw := b.bytes()
	raw = append(raw, 0x00) // simulate trailing junk

	_, err := ParseOtsTimestamp([]byte{0x00}, raw)
	if err == nil || !strings.Contains(err.Error(), "trailing bytes") {
		t.Fatalf("err = %v, want trailing-bytes error", err)
	}
}

func TestParseOtsTimestamp_RejectsTruncatedStream(t *testing.T) {
	var b otsBuilder
	b.opAppend([]byte("abc"))
	b.opSHA256()
	b.attBitcoin(1)
	raw := b.bytes()

	for _, cut := range []int{1, 2, len(raw) - 1} {
		_, err := ParseOtsTimestamp([]byte{0x00}, raw[:cut])
		if err == nil {
			t.Fatalf("cut=%d: expected error, got nil", cut)
		}
	}
}

func TestParseOtsTimestamp_RejectsUnsupportedOp(t *testing.T) {
	raw := []byte{0x42}
	_, err := ParseOtsTimestamp([]byte{0x00}, raw)
	if err == nil || !strings.Contains(err.Error(), "unsupported op") {
		t.Fatalf("err = %v, want unsupported-op error", err)
	}
}

func TestParseOtsTimestamp_RejectsEmpty(t *testing.T) {
	if _, err := ParseOtsTimestamp([]byte{}, []byte{0x00}); err == nil {
		t.Fatal("empty initial digest should error")
	}
	if _, err := ParseOtsTimestamp([]byte{0x00}, []byte{}); err == nil {
		t.Fatal("empty body should error")
	}
}

func TestParseOtsTimestamp_RejectsBreadthDoS(t *testing.T) {
	// Construct otsMaxAttestationsPerProof+1 sibling attestations.
	var b otsBuilder
	for i := 0; i < otsMaxAttestationsPerProof+1; i++ {
		if i < otsMaxAttestationsPerProof {
			b.sibling()
		}
		b.attPending("x")
	}
	_, err := ParseOtsTimestamp([]byte{0x00}, b.bytes())
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("err = %v, want breadth-limit error", err)
	}
}

func TestParseOtsTimestamp_RejectsDepthDoS(t *testing.T) {
	// otsMaxRecursionDepth+1 levels of sha256 followed by a leaf.
	var b otsBuilder
	for i := 0; i < otsMaxRecursionDepth+1; i++ {
		b.opSHA256()
	}
	b.attBitcoin(1)
	_, err := ParseOtsTimestamp([]byte{0x00}, b.bytes())
	if err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("err = %v, want depth-limit error", err)
	}
}

func TestParseOtsTimestamp_VaruintRoundTrip(t *testing.T) {
	// Sanity: multi-byte block heights encode/decode losslessly via
	// the builder + parser pair. 800123 sits comfortably above the
	// single-byte threshold (>127) so this exercises the
	// continuation-bit path in both directions.
	var b otsBuilder
	b.attBitcoin(800123)

	got, err := ParseOtsTimestamp([]byte{0x00}, b.bytes())
	if err != nil {
		t.Fatalf("ParseOtsTimestamp: %v", err)
	}
	if got.Attestations[0].BlockHeight != 800123 {
		t.Fatalf("height = %d, want 800123", got.Attestations[0].BlockHeight)
	}
}

func TestParseOtsTimestamp_NilSafe(t *testing.T) {
	var p *OtsParsedTimestamp
	if p.HasBitcoin() {
		t.Fatal("nil HasBitcoin() = true")
	}
	if p.HasPending() {
		t.Fatal("nil HasPending() = true")
	}
	if got := p.BitcoinAttestations(); got != nil {
		t.Fatalf("nil BitcoinAttestations() = %v, want nil", got)
	}
}
