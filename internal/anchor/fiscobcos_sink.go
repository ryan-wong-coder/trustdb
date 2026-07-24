package anchor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/wowtrust/trustdb/internal/anchor/fiscobcos"
	"github.com/wowtrust/trustdb/internal/cryptosuite"
	"github.com/wowtrust/trustdb/internal/model"
	"github.com/wowtrust/trustdb/internal/observability"
)

const minimumFISCOBCOSEndpoints = 2

type FISCOBCOSStandardSinkConfig struct {
	TrustConfig fiscobcos.TrustConfig
	Drivers     []fiscobcos.Driver
	Metrics     *observability.Metrics
	Logger      zerolog.Logger
	Clock       func() time.Time
}

// FISCOBCOSStandardSink owns quorum reads and exact payload/result binding.
// Drivers own network and SDK details. A successful Publish always includes
// transaction and receipt proof fields, an exact contract readback, the
// containing header, and a consensus snapshot; submission alone is never
// reported as an L5 result.
type FISCOBCOSStandardSink struct {
	trust   fiscobcos.TrustConfig
	drivers []fiscobcos.Driver
	metrics *observability.Metrics
	logger  zerolog.Logger
	clock   func() time.Time

	closeOnce sync.Once
	closeErr  error
}

func NewFISCOBCOSStandardSink(config FISCOBCOSStandardSinkConfig) (*FISCOBCOSStandardSink, error) {
	canonicalBytes, err := fiscobcos.MarshalTrustConfig(config.TrustConfig)
	if err != nil {
		return nil, err
	}
	trust, err := fiscobcos.UnmarshalTrustConfig(canonicalBytes)
	if err != nil {
		return nil, err
	}
	if trust.CryptoMode != fiscobcos.CryptoModeStandard {
		return nil, fmt.Errorf("%w: standard sink requires crypto_mode=standard", fiscobcos.ErrWrongNetwork)
	}
	if len(config.Drivers) < minimumFISCOBCOSEndpoints || trust.ReadQuorum < minimumFISCOBCOSEndpoints {
		return nil, fmt.Errorf("%w: standard sink requires at least two endpoints and read_quorum >= 2", fiscobcos.ErrDriverInvalid)
	}
	if int(trust.ReadQuorum) > len(config.Drivers) {
		return nil, fmt.Errorf("%w: read_quorum exceeds driver count", fiscobcos.ErrDriverInvalid)
	}
	drivers := append([]fiscobcos.Driver(nil), config.Drivers...)
	expected := make(map[string]struct{}, len(trust.Endpoints))
	for _, endpoint := range trust.Endpoints {
		expected[endpoint] = struct{}{}
	}
	seen := make(map[string]struct{}, len(drivers))
	for _, driver := range drivers {
		if driver == nil || strings.TrimSpace(driver.Endpoint()) == "" {
			return nil, fmt.Errorf("%w: nil driver or empty endpoint", fiscobcos.ErrDriverInvalid)
		}
		endpoint := driver.Endpoint()
		if _, ok := expected[endpoint]; !ok {
			return nil, fmt.Errorf("%w: driver endpoint %q is not in TrustConfig", fiscobcos.ErrWrongNetwork, endpoint)
		}
		if _, duplicate := seen[endpoint]; duplicate {
			return nil, fmt.Errorf("%w: duplicate driver endpoint %q", fiscobcos.ErrDriverInvalid, endpoint)
		}
		seen[endpoint] = struct{}{}
	}
	if len(seen) != len(expected) {
		return nil, fmt.Errorf("%w: every TrustConfig endpoint requires exactly one driver", fiscobcos.ErrDriverInvalid)
	}
	sort.Slice(drivers, func(i, j int) bool { return drivers[i].Endpoint() < drivers[j].Endpoint() })
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &FISCOBCOSStandardSink{
		trust: trust, drivers: drivers, metrics: config.Metrics, logger: config.Logger, clock: config.Clock,
	}, nil
}

func (*FISCOBCOSStandardSink) Name() string { return fiscobcos.SinkName }

// Probe validates every configured endpoint before a side effect. Requiring
// exact height equality is conservative by design for #464; bounded stale
// reads and quorum-height policy belong to the retry hardening in #470.
func (s *FISCOBCOSStandardSink) Probe(ctx context.Context) ([]fiscobcos.ChainProbe, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: nil standard sink", fiscobcos.ErrDriverInvalid)
	}
	type result struct {
		probe fiscobcos.ChainProbe
		err   error
	}
	results := make(chan result, len(s.drivers))
	for _, driver := range s.drivers {
		driver := driver
		go func() {
			probe, err := driver.ProbeChain(ctx)
			results <- result{probe: probe, err: err}
		}()
	}
	probes := make([]fiscobcos.ChainProbe, 0, len(s.drivers))
	for range s.drivers {
		item := <-results
		if item.err != nil {
			return nil, classifyDriverFailure("probe", item.probe.Endpoint, item.err)
		}
		if item.probe.Endpoint == "" {
			return nil, permanentDriverFailure("probe", "", fiscobcos.ErrDriverInvalid)
		}
		if err := validateProbeForSink(item.probe, s.trust); err != nil {
			return nil, err
		}
		probes = append(probes, cloneChainProbe(item.probe))
	}
	sort.Slice(probes, func(i, j int) bool { return probes[i].Endpoint < probes[j].Endpoint })
	for i := 1; i < len(probes); i++ {
		if !sameChainProbe(probes[0], probes[i]) {
			return nil, permanentDriverFailure("probe", probes[i].Endpoint, fiscobcos.ErrEndpointDisagreement)
		}
	}
	return probes, nil
}

func (s *FISCOBCOSStandardSink) Publish(ctx context.Context, sth model.SignedTreeHead) (model.STHAnchorResult, error) {
	if _, err := s.Probe(ctx); err != nil {
		return model.STHAnchorResult{}, mapSinkError(err)
	}
	payload, err := payloadForSTH(sth)
	if err != nil {
		return model.STHAnchorResult{}, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	canonicalPayload, err := fiscobcos.MarshalPayload(payload)
	if err != nil {
		return model.STHAnchorResult{}, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	request := fiscobcos.SubmitRequest{Payload: payload, CanonicalPayload: canonicalPayload}
	submission, err := s.drivers[0].SubmitAnchor(ctx, request)
	if err != nil {
		return model.STHAnchorResult{}, mapSinkError(classifyDriverFailure("submit_anchor", s.drivers[0].Endpoint(), err))
	}
	if err := validateTransactionAttempt(submission.Attempt); err != nil {
		return model.STHAnchorResult{}, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	records, err := s.readAnchorQuorum(ctx, payload)
	if err != nil {
		return model.STHAnchorResult{}, mapSinkError(err)
	}
	receipt, err := s.drivers[0].GetReceiptWithProof(ctx, submission.Attempt.TransactionHash)
	if err != nil {
		return model.STHAnchorResult{}, mapSinkError(classifyDriverFailure("get_receipt_with_proof", s.drivers[0].Endpoint(), err))
	}
	if receipt.Status != fiscobcos.ReceiptStatusOK {
		return model.STHAnchorResult{}, fmt.Errorf("%w: %w", ErrPermanent, fiscobcos.ErrInvalidReceiptStatus)
	}
	if err := validateReceipt(payload, submission.Attempt, receipt, records[0]); err != nil {
		return model.STHAnchorResult{}, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	header, consensus, err := s.readBlockQuorum(ctx, receipt.BlockNumber, receipt.BlockHash)
	if err != nil {
		return model.STHAnchorResult{}, mapSinkError(err)
	}
	proof, err := s.buildProof(canonicalPayload, submission.Attempt, receipt, header, consensus)
	if err != nil {
		return model.STHAnchorResult{}, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	proofBytes, err := fiscobcos.MarshalProof(proof)
	if err != nil {
		return model.STHAnchorResult{}, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	return model.STHAnchorResult{
		SchemaVersion: model.SchemaSTHAnchorResult,
		NodeID:        sth.NodeID, LogID: sth.LogID, TreeSize: sth.TreeSize,
		SinkName: fiscobcos.SinkName, AnchorID: fiscobcos.AnchorIDString(payload),
		RootHash: append([]byte(nil), sth.RootHash...), STH: sth, Proof: proofBytes,
		PublishedAtUnixN: s.clock().UTC().UnixNano(),
	}, nil
}

func (s *FISCOBCOSStandardSink) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		var errs []error
		for _, driver := range s.drivers {
			if err := driver.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close FISCO BCOS endpoint driver: %w", err))
			}
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}

func (s *FISCOBCOSStandardSink) readAnchorQuorum(ctx context.Context, payload fiscobcos.AnchorPayload) ([]fiscobcos.AnchorRecord, error) {
	quorum := int(s.trust.ReadQuorum)
	records := make([]fiscobcos.AnchorRecord, 0, quorum)
	for _, driver := range s.drivers[:quorum] {
		record, err := driver.ReadAnchor(ctx, payload.AnchorID)
		if err != nil {
			return nil, classifyDriverFailure("read_anchor", driver.Endpoint(), err)
		}
		if err := fiscobcos.ValidateAnchorRecord(payload, record); err != nil {
			return nil, permanentDriverFailure("read_anchor", driver.Endpoint(), err)
		}
		if len(records) > 0 && !sameAnchorRecord(records[0], record) {
			return nil, permanentDriverFailure("read_anchor", driver.Endpoint(), fiscobcos.ErrEndpointDisagreement)
		}
		records = append(records, cloneAnchorRecord(record))
	}
	return records, nil
}

func (s *FISCOBCOSStandardSink) readBlockQuorum(ctx context.Context, blockNumber uint64, blockHash []byte) (fiscobcos.BlockHeader, fiscobcos.ConsensusSnapshot, error) {
	quorum := int(s.trust.ReadQuorum)
	var selectedHeader fiscobcos.BlockHeader
	var selectedConsensus fiscobcos.ConsensusSnapshot
	for index, driver := range s.drivers[:quorum] {
		header, err := driver.GetBlockHeader(ctx, blockNumber)
		if err != nil {
			return fiscobcos.BlockHeader{}, fiscobcos.ConsensusSnapshot{}, classifyDriverFailure("get_block_header", driver.Endpoint(), err)
		}
		consensus, err := driver.GetConsensusSnapshot(ctx, blockNumber)
		if err != nil {
			return fiscobcos.BlockHeader{}, fiscobcos.ConsensusSnapshot{}, classifyDriverFailure("get_consensus_snapshot", driver.Endpoint(), err)
		}
		if err := validateBlockObservation(blockNumber, blockHash, header, consensus); err != nil {
			return fiscobcos.BlockHeader{}, fiscobcos.ConsensusSnapshot{}, permanentDriverFailure("read_block", driver.Endpoint(), err)
		}
		if index == 0 {
			selectedHeader = cloneBlockHeader(header)
			selectedConsensus = cloneConsensus(consensus)
			continue
		}
		if !sameBlockHeader(selectedHeader, header) || !sameConsensusSnapshot(selectedConsensus, consensus) {
			return fiscobcos.BlockHeader{}, fiscobcos.ConsensusSnapshot{}, permanentDriverFailure("read_block", driver.Endpoint(), fiscobcos.ErrEndpointDisagreement)
		}
	}
	return selectedHeader, selectedConsensus, nil
}

func (s *FISCOBCOSStandardSink) buildProof(payload []byte, attempt fiscobcos.TransactionAttempt, receipt fiscobcos.ReceiptWithProof, header fiscobcos.BlockHeader, consensus fiscobcos.ConsensusSnapshot) (fiscobcos.AnchorProof, error) {
	contextID, err := fiscobcos.ChainContextID(s.trust)
	if err != nil {
		return fiscobcos.AnchorProof{}, err
	}
	return fiscobcos.AnchorProof{
		SchemaVersion: fiscobcos.SchemaAnchorProof, FormatVersion: fiscobcos.ProofVersion,
		CryptoMode:              s.trust.CryptoMode,
		ProtocolHashAlgorithm:   s.trust.ProtocolHashAlgorithm,
		ChainHashAlgorithm:      s.trust.ChainHashAlgorithm,
		ChainSignatureAlgorithm: s.trust.ChainSignatureAlgorithm,
		ChainID:                 s.trust.ChainID, GroupID: s.trust.GroupID,
		GenesisHash:               append([]byte(nil), s.trust.GenesisHash...),
		TrustedCheckpoint:         s.trust.TrustedCheckpoint,
		Contract:                  s.trust.Contract,
		ChainContextID:            contextID,
		CanonicalPayload:          append([]byte(nil), payload...),
		TransactionAttempts:       []fiscobcos.TransactionAttempt{attempt},
		SuccessfulTransactionHash: append([]byte(nil), attempt.TransactionHash...),
		Receipt:                   receipt.Evidence,
		Block:                     header.Evidence,
		Finality:                  consensus.Finality,
	}, nil
}

func payloadForSTH(sth model.SignedTreeHead) (fiscobcos.AnchorPayload, error) {
	var matched []fiscobcos.AnchorPayload
	for _, suite := range []cryptosuite.ID{cryptosuite.INTLV1, cryptosuite.CNSMV1} {
		payload, err := fiscobcos.NewAnchorPayload(suite, sth)
		if err == nil {
			matched = append(matched, payload)
		}
	}
	if len(matched) != 1 {
		return fiscobcos.AnchorPayload{}, fmt.Errorf("%w: signed STH matches %d suites", fiscobcos.ErrInvalidPayload, len(matched))
	}
	return matched[0], nil
}

func validateTransactionAttempt(attempt fiscobcos.TransactionAttempt) error {
	if len(attempt.RawCanonicalTransaction) == 0 || len(attempt.Signature) == 0 ||
		len(attempt.Sender) == 0 || len(attempt.TransactionHash) != 32 ||
		attempt.BlockLimit == 0 || attempt.SubmittedAtUnixN <= 0 {
		return fiscobcos.ErrIncompleteChainEvidence
	}
	return nil
}

func validateReceipt(payload fiscobcos.AnchorPayload, attempt fiscobcos.TransactionAttempt, receipt fiscobcos.ReceiptWithProof, quorumRecord fiscobcos.AnchorRecord) error {
	if receipt.BlockNumber == 0 || len(receipt.BlockHash) != 32 ||
		!bytes.Equal(receipt.Evidence.TransactionHash, attempt.TransactionHash) ||
		len(receipt.Evidence.RawCanonicalReceipt) == 0 ||
		len(receipt.Evidence.ReceiptHash) != 32 ||
		len(receipt.Evidence.DecodedAnchorEvent) == 0 ||
		receipt.Evidence.TransactionProof == nil ||
		receipt.Evidence.ReceiptProof == nil {
		return fiscobcos.ErrIncompleteChainEvidence
	}
	if err := fiscobcos.ValidateAnchorRecord(payload, receipt.Record); err != nil {
		return err
	}
	if !sameAnchorRecord(receipt.Record, quorumRecord) {
		return fiscobcos.ErrEndpointDisagreement
	}
	return nil
}

func validateBlockObservation(blockNumber uint64, blockHash []byte, header fiscobcos.BlockHeader, consensus fiscobcos.ConsensusSnapshot) error {
	if blockNumber == 0 || header.Evidence.BlockNumber != blockNumber ||
		len(header.Evidence.RawCanonicalHeader) == 0 ||
		!bytes.Equal(header.Evidence.BlockHash, blockHash) ||
		consensus.BlockNumber != blockNumber ||
		!bytes.Equal(consensus.BlockHash, blockHash) ||
		len(consensus.Finality.Signatures) == 0 {
		return fiscobcos.ErrIncompleteChainEvidence
	}
	return nil
}

func validateProbeForSink(probe fiscobcos.ChainProbe, trust fiscobcos.TrustConfig) error {
	if probe.SDKVersion != fiscobcos.StandardSDKVersion {
		return permanentDriverFailure("probe", probe.Endpoint, fiscobcos.ErrUnsupportedSDK)
	}
	if probe.CryptoMode != fiscobcos.CryptoModeStandard ||
		probe.ChainID != trust.ChainID || probe.GroupID != trust.GroupID ||
		!bytes.Equal(probe.GenesisHash, trust.GenesisHash) ||
		!bytes.Equal(probe.CheckpointHash, trust.TrustedCheckpoint.BlockHash) {
		return permanentDriverFailure("probe", probe.Endpoint, fiscobcos.ErrWrongNetwork)
	}
	if probe.Height < trust.TrustedCheckpoint.BlockNumber {
		return &fiscobcos.DriverError{Operation: "probe", Endpoint: probe.Endpoint, Class: fiscobcos.FailureTransient, Kind: fiscobcos.ErrStaleEndpoint}
	}
	if !bytes.Equal(probe.ContractCodeHash, trust.Contract.CodeHash) {
		return permanentDriverFailure("probe", probe.Endpoint, fiscobcos.ErrContractMismatch)
	}
	return nil
}

func sameChainProbe(left, right fiscobcos.ChainProbe) bool {
	return left.SDKVersion == right.SDKVersion && left.CryptoMode == right.CryptoMode &&
		left.ChainID == right.ChainID && left.GroupID == right.GroupID &&
		bytes.Equal(left.GenesisHash, right.GenesisHash) &&
		bytes.Equal(left.CheckpointHash, right.CheckpointHash) &&
		left.Height == right.Height &&
		bytes.Equal(left.ContractCodeHash, right.ContractCodeHash)
}

func sameAnchorRecord(left, right fiscobcos.AnchorRecord) bool {
	return bytes.Equal(left.StreamID, right.StreamID) && left.TreeSize == right.TreeSize &&
		bytes.Equal(left.RootHash, right.RootHash) &&
		bytes.Equal(left.SignedSTHDigest, right.SignedSTHDigest) &&
		bytes.Equal(left.Publisher, right.Publisher) &&
		left.PayloadVersion == right.PayloadVersion && left.Exists == right.Exists
}

func sameBlockHeader(left, right fiscobcos.BlockHeader) bool {
	return left.Evidence.BlockNumber == right.Evidence.BlockNumber &&
		bytes.Equal(left.Evidence.BlockHash, right.Evidence.BlockHash) &&
		bytes.Equal(left.Evidence.RawCanonicalHeader, right.Evidence.RawCanonicalHeader)
}

func sameConsensusSnapshot(left, right fiscobcos.ConsensusSnapshot) bool {
	if left.BlockNumber != right.BlockNumber || !bytes.Equal(left.BlockHash, right.BlockHash) ||
		left.Finality.View != right.Finality.View || left.Finality.Round != right.Finality.Round ||
		len(left.Finality.Signatures) != len(right.Finality.Signatures) {
		return false
	}
	for i := range left.Finality.Signatures {
		if left.Finality.Signatures[i].ValidatorNodeID != right.Finality.Signatures[i].ValidatorNodeID ||
			!bytes.Equal(left.Finality.Signatures[i].Signature, right.Finality.Signatures[i].Signature) {
			return false
		}
	}
	return true
}

func permanentDriverFailure(operation, endpoint string, kind error) error {
	return &fiscobcos.DriverError{Operation: operation, Endpoint: endpoint, Class: fiscobcos.FailurePermanent, Kind: kind}
}

func classifyDriverFailure(operation, endpoint string, err error) error {
	if err == nil {
		return nil
	}
	var classified *fiscobcos.DriverError
	if errors.As(err, &classified) {
		return err
	}
	return &fiscobcos.DriverError{Operation: operation, Endpoint: endpoint, Class: fiscobcos.FailureTransient, Kind: err}
}

func mapSinkError(err error) error {
	if fiscobcos.IsPermanentDriverError(err) {
		return fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	return err
}

func cloneChainProbe(in fiscobcos.ChainProbe) fiscobcos.ChainProbe {
	in.GenesisHash = append([]byte(nil), in.GenesisHash...)
	in.CheckpointHash = append([]byte(nil), in.CheckpointHash...)
	in.ContractCodeHash = append([]byte(nil), in.ContractCodeHash...)
	return in
}

func cloneAnchorRecord(in fiscobcos.AnchorRecord) fiscobcos.AnchorRecord {
	in.StreamID = append([]byte(nil), in.StreamID...)
	in.RootHash = append([]byte(nil), in.RootHash...)
	in.SignedSTHDigest = append([]byte(nil), in.SignedSTHDigest...)
	in.Publisher = append([]byte(nil), in.Publisher...)
	return in
}

func cloneBlockHeader(in fiscobcos.BlockHeader) fiscobcos.BlockHeader {
	in.Evidence.RawCanonicalHeader = append([]byte(nil), in.Evidence.RawCanonicalHeader...)
	in.Evidence.BlockHash = append([]byte(nil), in.Evidence.BlockHash...)
	return in
}

func cloneConsensus(in fiscobcos.ConsensusSnapshot) fiscobcos.ConsensusSnapshot {
	in.BlockHash = append([]byte(nil), in.BlockHash...)
	in.Finality.Signatures = append([]fiscobcos.CommitSignature(nil), in.Finality.Signatures...)
	for i := range in.Finality.Signatures {
		in.Finality.Signatures[i].Signature = append([]byte(nil), in.Finality.Signatures[i].Signature...)
	}
	return in
}
