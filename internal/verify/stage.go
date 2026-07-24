package verify

import (
	"errors"
)

// Stage identifies the fail-closed verification boundary that rejected an
// input. The wrapped error text remains unchanged so existing callers keep
// stable diagnostics while offline verifiers can report structured progress.
type Stage string

const (
	StageProofBundle      Stage = "proof_bundle"
	StageContent          Stage = "content"
	StageClientClaim      Stage = "client_claim"
	StageBundleBindings   Stage = "bundle_bindings"
	StageAcceptedReceipt  Stage = "accepted_receipt"
	StageCommittedReceipt Stage = "committed_receipt"
	StageBatchMerkle      Stage = "batch_merkle"
	StageGlobalLog        Stage = "global_log"
	StageAnchor           Stage = "anchor"
)

type StageError struct {
	Stage Stage
	Err   error
}

func (e *StageError) Error() string {
	if e == nil || e.Err == nil {
		return "verify: verification stage failed"
	}
	return e.Err.Error()
}

func (e *StageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func failStage(stage Stage, err error) error {
	if err == nil {
		err = errors.New("verify: verification stage failed")
	}
	return &StageError{Stage: stage, Err: err}
}

// FailedStage extracts a structured stage without depending on error text.
func FailedStage(err error) (Stage, bool) {
	var stageError *StageError
	if !errors.As(err, &stageError) || stageError == nil || stageError.Stage == "" {
		return "", false
	}
	return stageError.Stage, true
}
