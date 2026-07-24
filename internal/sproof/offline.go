package sproof

import (
	"fmt"
	"io"

	"github.com/wowtrust/trustdb/internal/model"
	"github.com/wowtrust/trustdb/internal/verify"
)

type OfflineStageStatus string

const (
	OfflineStagePassed     OfflineStageStatus = "passed"
	OfflineStageFailed     OfflineStageStatus = "failed"
	OfflineStageNotPresent OfflineStageStatus = "not_present"
	OfflineStageSkipped    OfflineStageStatus = "skipped"
	OfflineStageNotRun     OfflineStageStatus = "not_run"
)

const (
	OfflineStageContainer = "sproof_container"
	OfflineStageIdentity  = "identity_evidence"
)

// OfflineStageResult is a deterministic account of each local verification
// boundary. A failed stage prevents every later applicable stage from running.
type OfflineStageResult struct {
	Name   string             `json:"name"`
	Status OfflineStageStatus `json:"status"`
	Error  string             `json:"error,omitempty"`
}

// OfflineResult reports only independently recomputed values. ProofLevel is
// never copied from the descriptive label inside the evidence file.
type OfflineResult struct {
	Valid                  bool                 `json:"valid"`
	RecordID               string               `json:"record_id,omitempty"`
	ProofLevel             string               `json:"proof_level,omitempty"`
	AnchorSink             string               `json:"anchor_sink,omitempty"`
	AnchorID               string               `json:"anchor_id,omitempty"`
	Identity               IdentityReport       `json:"identity"`
	Stages                 []OfflineStageResult `json:"stages"`
	ExternalNetworkAccess  bool                 `json:"external_network_access"`
	ExternalProviderAccess bool                 `json:"external_provider_access"`
}

type OfflineTrust struct {
	Proof    verify.TrustedKeys
	Identity IdentityTrust
}

type OfflineOptions struct {
	SkipAnchor bool
}

// VerifyOffline verifies a complete .sproof without server, CA, provider, DNS,
// or network access. Every trust root must already be present in trust.
func VerifyOffline(
	raw io.Reader,
	proof model.SingleProof,
	trust OfflineTrust,
	options OfflineOptions,
) (OfflineResult, error) {
	result := OfflineResult{
		RecordID: proof.RecordID,
		Stages:   make([]OfflineStageResult, 0, 11),
	}
	if err := Validate(proof); err != nil {
		result.Stages = append(result.Stages, failedOfflineStage(OfflineStageContainer, err))
		appendNotRunProofStages(&result, proof, options)
		return result, err
	}
	result.Stages = append(result.Stages, passedOfflineStage(OfflineStageContainer))

	identityReport, err := VerifyIdentityEvidence(proof, trust.Identity)
	if err != nil {
		result.Stages = append(result.Stages, failedOfflineStage(OfflineStageIdentity, err))
		appendNotRunProofStages(&result, proof, options)
		return result, err
	}
	result.Identity = identityReport
	if len(proof.IdentityEvidence) == 0 {
		result.Stages = append(result.Stages, OfflineStageResult{
			Name:   OfflineStageIdentity,
			Status: OfflineStageNotPresent,
		})
	} else {
		result.Stages = append(result.Stages, passedOfflineStage(OfflineStageIdentity))
	}

	verifyOptions := make([]verify.Option, 0, 3)
	if proof.GlobalProof != nil {
		verifyOptions = append(verifyOptions, verify.WithGlobalProof(*proof.GlobalProof))
	}
	if proof.AnchorResult != nil && !options.SkipAnchor {
		verifyOptions = append(verifyOptions, verify.WithAnchor(*proof.AnchorResult))
	}
	verified, err := verify.ProofBundle(raw, proof.ProofBundle, trust.Proof, verifyOptions...)
	if err != nil {
		failed, ok := verify.FailedStage(err)
		if !ok {
			failed = verify.StageProofBundle
		}
		appendProofStages(&result, proof, options, failed, err)
		return result, err
	}
	appendProofStages(&result, proof, options, "", nil)
	result.Valid = verified.Valid
	result.RecordID = verified.RecordID
	result.ProofLevel = verified.ProofLevel
	result.AnchorSink = verified.AnchorSink
	result.AnchorID = verified.AnchorID
	return result, nil
}

var orderedProofStages = []verify.Stage{
	verify.StageProofBundle,
	verify.StageContent,
	verify.StageClientClaim,
	verify.StageBundleBindings,
	verify.StageAcceptedReceipt,
	verify.StageCommittedReceipt,
	verify.StageBatchMerkle,
	verify.StageGlobalLog,
	verify.StageAnchor,
}

func appendNotRunProofStages(result *OfflineResult, proof model.SingleProof, options OfflineOptions) {
	appendProofStages(result, proof, options, verify.StageProofBundle, fmt.Errorf("verification did not reach the proof bundle"))
	if len(result.Stages) != 0 {
		for index := range result.Stages {
			if result.Stages[index].Name == string(verify.StageProofBundle) {
				result.Stages[index].Status = OfflineStageNotRun
				result.Stages[index].Error = ""
				break
			}
		}
	}
}

func appendProofStages(
	result *OfflineResult,
	proof model.SingleProof,
	options OfflineOptions,
	failed verify.Stage,
	failure error,
) {
	reachedFailure := false
	for _, stage := range orderedProofStages {
		switch stage {
		case verify.StageGlobalLog:
			if proof.GlobalProof == nil {
				result.Stages = append(result.Stages, OfflineStageResult{
					Name:   string(stage),
					Status: OfflineStageNotPresent,
				})
				continue
			}
		case verify.StageAnchor:
			if proof.AnchorResult == nil {
				result.Stages = append(result.Stages, OfflineStageResult{
					Name:   string(stage),
					Status: OfflineStageNotPresent,
				})
				continue
			}
			if options.SkipAnchor {
				result.Stages = append(result.Stages, OfflineStageResult{
					Name:   string(stage),
					Status: OfflineStageSkipped,
				})
				continue
			}
		}
		if failed != "" && stage == failed {
			result.Stages = append(result.Stages, failedOfflineStage(string(stage), failure))
			reachedFailure = true
			continue
		}
		status := OfflineStagePassed
		if reachedFailure {
			status = OfflineStageNotRun
		}
		result.Stages = append(result.Stages, OfflineStageResult{
			Name:   string(stage),
			Status: status,
		})
	}
}

func passedOfflineStage(name string) OfflineStageResult {
	return OfflineStageResult{Name: name, Status: OfflineStagePassed}
}

func failedOfflineStage(name string, err error) OfflineStageResult {
	stage := OfflineStageResult{Name: name, Status: OfflineStageFailed}
	if err != nil {
		stage.Error = err.Error()
	}
	return stage
}
