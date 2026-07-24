package fiscobcos

import (
	"bytes"
	"fmt"

	"github.com/wowtrust/trustdb/internal/cborx"
	"github.com/wowtrust/trustdb/internal/model"
)

const (
	SchemaPublicationObservation   = "trustdb.fisco-bcos-publication-observation.v1"
	MaxPublicationObservationBytes = 16 << 20
)

// PublicationObservation is durable raw RPC material for an externally
// observed publication. It intentionally does not use AnchorProof's
// RawCanonical* fields and makes no receipt-inclusion or PBFT-finality claim.
// #465 may transform qualified native encodings into the offline proof format.
type PublicationObservation struct {
	SchemaVersion     string
	EvidenceStage     string
	CryptoMode        CryptoMode
	ChainID           string
	GroupID           string
	GenesisHash       []byte
	TrustedCheckpoint BlockCheckpoint
	Contract          ContractBinding
	ChainContextID    []byte
	CanonicalPayload  []byte
	Transaction       TransactionSubmission
	Receipt           ReceiptRPCObservation
	Event             AnchorPublishedEvent
	Readback          AnchorRecord
	Block             BlockRPCObservation
	Consensus         ConsensusSnapshot
}

func MarshalPublicationObservation(observation PublicationObservation) ([]byte, error) {
	if err := ValidatePublicationObservation(observation); err != nil {
		return nil, err
	}
	data, err := cborx.Marshal(observation)
	if err != nil {
		return nil, fmt.Errorf("%w: encode publication observation: %v", ErrDriverInvalid, err)
	}
	if len(data) > MaxPublicationObservationBytes {
		return nil, fmt.Errorf("%w: publication observation exceeds %d bytes", ErrDriverInvalid, MaxPublicationObservationBytes)
	}
	return data, nil
}

func UnmarshalPublicationObservation(data []byte) (PublicationObservation, error) {
	var observation PublicationObservation
	if err := cborx.UnmarshalLimit(data, &observation, MaxPublicationObservationBytes); err != nil {
		return PublicationObservation{}, fmt.Errorf("%w: decode publication observation: %v", ErrDriverInvalid, err)
	}
	if err := ValidatePublicationObservation(observation); err != nil {
		return PublicationObservation{}, err
	}
	return observation, nil
}

func ValidatePublicationObservation(observation PublicationObservation) error {
	if observation.SchemaVersion != SchemaPublicationObservation ||
		observation.EvidenceStage != model.AnchorEvidenceStageRaw ||
		observation.CryptoMode != CryptoModeStandard ||
		observation.ChainID == "" || observation.GroupID == "" ||
		len(observation.GenesisHash) != 32 ||
		len(observation.TrustedCheckpoint.BlockHash) != 32 ||
		len(observation.Contract.Address) != 20 ||
		len(observation.Contract.CodeHash) != 32 ||
		len(observation.ChainContextID) != 32 {
		return fmt.Errorf("%w: invalid publication observation identity", ErrDriverInvalid)
	}
	payload, err := UnmarshalPayload(observation.CanonicalPayload)
	if err != nil {
		return err
	}
	if len(observation.Transaction.EncodedTransaction) == 0 ||
		len(observation.Transaction.Signature) == 0 ||
		len(observation.Transaction.Sender) != 20 ||
		len(observation.Transaction.TransactionHash) != 32 ||
		observation.Transaction.BlockLimit == 0 ||
		observation.Transaction.SubmittedAtUnixN <= 0 {
		return fmt.Errorf("%w: incomplete transaction observation", ErrIncompleteChainEvidence)
	}
	if len(observation.Receipt.NormalizedRPCReceipt) == 0 ||
		observation.Receipt.Status != ReceiptStatusOK ||
		observation.Receipt.BlockNumber == 0 ||
		len(observation.Receipt.BlockHashClaim) != 32 ||
		len(observation.Receipt.ReceiptHashClaim) != 32 ||
		!bytes.Equal(observation.Receipt.TransactionHash, observation.Transaction.TransactionHash) ||
		observation.Receipt.TransactionProofRPC == nil ||
		observation.Receipt.ReceiptProofRPC == nil {
		return fmt.Errorf("%w: incomplete receipt RPC observation", ErrIncompleteChainEvidence)
	}
	event := observation.Event
	if !bytes.Equal(event.ContractAddress, observation.Contract.Address) ||
		!bytes.Equal(event.AnchorID, payload.AnchorID) ||
		!bytes.Equal(event.StreamID, payload.StreamID) ||
		event.TreeSize != payload.TreeSize ||
		!bytes.Equal(event.RootHash, payload.RootHash) ||
		!bytes.Equal(event.SignedSTHDigest, payload.SignedSTHDigest) ||
		!bytes.Equal(event.Publisher, observation.Transaction.Sender) ||
		event.PayloadVersion != payload.Version ||
		event.LogIndex != observation.Receipt.AnchorLogIndex ||
		len(event.NormalizedRPCLog) == 0 {
		return fmt.Errorf("%w: event observation does not match payload", ErrContractMismatch)
	}
	if err := ValidateAnchorRecord(payload, observation.Readback); err != nil {
		return err
	}
	if !bytes.Equal(observation.Readback.Publisher, event.Publisher) {
		return fmt.Errorf("%w: event publisher does not match contract readback", ErrContractMismatch)
	}
	if len(observation.Block.NormalizedRPCHeader) == 0 ||
		len(observation.Block.BlockHashClaim) != 32 ||
		observation.Block.BlockNumber == 0 ||
		observation.Receipt.BlockNumber != observation.Block.BlockNumber ||
		!bytes.Equal(observation.Receipt.BlockHashClaim, observation.Block.BlockHashClaim) ||
		observation.Consensus.BlockNumber != observation.Block.BlockNumber ||
		!bytes.Equal(observation.Consensus.BlockHash, observation.Block.BlockHashClaim) ||
		len(observation.Consensus.Finality.Signatures) == 0 {
		return fmt.Errorf("%w: incomplete block/consensus observation", ErrIncompleteChainEvidence)
	}
	return nil
}
