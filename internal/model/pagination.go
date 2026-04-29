package model

import "strings"

func NormalizedProofLevel(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "L1":
		return "L1"
	case "L2":
		return "L2"
	case "L3":
		return "L3"
	case "L4":
		return "L4"
	case "L5":
		return "L5"
	default:
		return ""
	}
}

func ProofLevelRank(raw string) int {
	switch NormalizedProofLevel(raw) {
	case "L1":
		return 1
	case "L2":
		return 2
	case "L3":
		return 3
	case "L4":
		return 4
	case "L5":
		return 5
	default:
		return 0
	}
}

func RecordIndexProofLevel(idx RecordIndex) string {
	if level := NormalizedProofLevel(idx.ProofLevel); level != "" {
		return level
	}
	if idx.BatchID != "" {
		return "L3"
	}
	return "L2"
}

func CompareBatchRootPosition(leftTime int64, leftBatchID string, rightTime int64, rightBatchID string) int {
	switch {
	case leftTime < rightTime:
		return -1
	case leftTime > rightTime:
		return 1
	case leftBatchID < rightBatchID:
		return -1
	case leftBatchID > rightBatchID:
		return 1
	default:
		return 0
	}
}

func BatchRootAfterCursor(root BatchRoot, opts RootListOptions) bool {
	if opts.AfterClosedAtUnixN == 0 && opts.AfterBatchID == "" {
		return true
	}
	cmp := CompareBatchRootPosition(root.ClosedAtUnixN, root.BatchID, opts.AfterClosedAtUnixN, opts.AfterBatchID)
	if strings.EqualFold(opts.Direction, RecordListDirectionAsc) {
		return cmp > 0
	}
	return cmp < 0
}

func CompareUint64Position(left, right uint64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func Uint64AfterCursor(value, after uint64, direction string) bool {
	if after == 0 {
		return true
	}
	cmp := CompareUint64Position(value, after)
	if strings.EqualFold(direction, RecordListDirectionAsc) {
		return cmp > 0
	}
	return cmp < 0
}
