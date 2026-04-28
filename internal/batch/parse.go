package batch

import (
	"strconv"
	"strings"
)

// batchIDPrefix is the literal head of every batch_id produced by
// nextBatchID. It is duplicated here rather than referenced from the
// fmt.Sprintf in service.go because ParseBatchSeq has to remain
// usable from external callers (the serve startup path that restores
// the seq counter) without dragging in the rest of the Service.
const batchIDPrefix = "batch-"

// ParseBatchSeq extracts the trailing "-NNNNNN" sequence number from
// a batch_id formatted as batch-<unix_nano>-<seq>.
//
// Returns (0, false) for any input that doesn't match the format.
// Callers MUST treat this as best-effort: a malformed or empty id
// (typical on a brand-new deployment with no prior roots) means the
// caller should fall back to seq=0 rather than refuse to start.
//
// The function is intentionally permissive about the unix_nano part —
// we only validate that it parses as a non-empty number — because
// the seq is the only field we care about restoring across restarts;
// the timestamp half is not used by anything in the system.
func ParseBatchSeq(id string) (uint64, bool) {
	if !strings.HasPrefix(id, batchIDPrefix) {
		return 0, false
	}
	body := id[len(batchIDPrefix):]
	// body should look like "<unix_nano>-<seq>". Splitting on the
	// last "-" lets the timestamp half contain whatever digits it
	// wants without us having to know in advance how long it is.
	dash := strings.LastIndexByte(body, '-')
	if dash <= 0 || dash == len(body)-1 {
		return 0, false
	}
	tsPart := body[:dash]
	seqPart := body[dash+1:]
	if _, err := strconv.ParseUint(tsPart, 10, 64); err != nil {
		return 0, false
	}
	seq, err := strconv.ParseUint(seqPart, 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}
