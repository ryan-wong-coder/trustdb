package anchor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/wowtrust/trustdb/internal/anchor/fiscobcos"
	"github.com/wowtrust/trustdb/internal/cryptosuite"
	"github.com/wowtrust/trustdb/internal/model"
	"github.com/wowtrust/trustdb/internal/proofstore"
)

type fakeBCOSState struct {
	mu                  sync.Mutex
	record              fiscobcos.AnchorRecord
	attempt             fiscobcos.TransactionAttempt
	receipt             fiscobcos.ReceiptWithProof
	submitCalls         int
	failAfterEffectOnce bool
}

type fakeBCOSDriver struct {
	endpoint string
	probe    fiscobcos.ChainProbe
	state    *fakeBCOSState
	closed   bool
}

func (d *fakeBCOSDriver) Endpoint() string { return d.endpoint }
func (d *fakeBCOSDriver) ProbeChain(context.Context) (fiscobcos.ChainProbe, error) {
	return cloneChainProbe(d.probe), nil
}
func (d *fakeBCOSDriver) SubmitAnchor(_ context.Context, request fiscobcos.SubmitRequest) (fiscobcos.Submission, error) {
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	d.state.submitCalls++
	txHash := sha256.Sum256(append(append([]byte(nil), request.Payload.AnchorID...), byte(d.state.submitCalls)))
	d.state.attempt = fiscobcos.TransactionAttempt{
		RawCanonicalTransaction: append([]byte("canonical-transaction-"), txHash[:]...),
		Signature:               bytes.Repeat([]byte{0x51}, 65),
		Sender:                  bytes.Repeat([]byte{0x61}, 20),
		TransactionHash:         txHash[:],
		BlockLimit:              700,
		SubmittedAtUnixN:        int64(d.state.submitCalls),
	}
	d.state.record = fiscobcos.AnchorRecord{
		StreamID: append([]byte(nil), request.Payload.StreamID...), TreeSize: request.Payload.TreeSize,
		RootHash:        append([]byte(nil), request.Payload.RootHash...),
		SignedSTHDigest: append([]byte(nil), request.Payload.SignedSTHDigest...),
		Publisher:       bytes.Repeat([]byte{0x61}, 20), PayloadVersion: request.Payload.Version, Exists: true,
	}
	blockHash := bytes.Repeat([]byte{0x71}, 32)
	d.state.receipt = fiscobcos.ReceiptWithProof{
		Status: fiscobcos.ReceiptStatusOK, BlockNumber: 500, BlockHash: blockHash, Record: cloneAnchorRecord(d.state.record),
		Evidence: fiscobcos.ReceiptEvidence{
			RawCanonicalReceipt: []byte("canonical-receipt"), ReceiptHash: bytes.Repeat([]byte{0x72}, 32),
			TransactionHash: txHash[:], TransactionProof: [][]byte{}, ReceiptProof: [][]byte{},
			DecodedAnchorEvent: []byte("exact-anchor-event"),
		},
	}
	if d.state.failAfterEffectOnce {
		d.state.failAfterEffectOnce = false
		return fiscobcos.Submission{}, errors.New("connection lost after submission")
	}
	return fiscobcos.Submission{Attempt: cloneAttempt(d.state.attempt)}, nil
}
func (d *fakeBCOSDriver) ReadAnchor(context.Context, []byte) (fiscobcos.AnchorRecord, error) {
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	return cloneAnchorRecord(d.state.record), nil
}
func (d *fakeBCOSDriver) GetReceiptWithProof(context.Context, []byte) (fiscobcos.ReceiptWithProof, error) {
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	return cloneReceipt(d.state.receipt), nil
}
func (d *fakeBCOSDriver) GetBlockHeader(context.Context, uint64) (fiscobcos.BlockHeader, error) {
	return fiscobcos.BlockHeader{Evidence: fiscobcos.BlockEvidence{
		RawCanonicalHeader: []byte("canonical-header"), BlockHash: bytes.Repeat([]byte{0x71}, 32), BlockNumber: 500,
	}}, nil
}
func (d *fakeBCOSDriver) GetConsensusSnapshot(context.Context, uint64) (fiscobcos.ConsensusSnapshot, error) {
	return fiscobcos.ConsensusSnapshot{
		BlockNumber: 500, BlockHash: bytes.Repeat([]byte{0x71}, 32),
		Finality: fiscobcos.FinalityEvidence{View: 9, Round: 2, Signatures: []fiscobcos.CommitSignature{
			{ValidatorNodeID: "validator-a", Signature: bytes.Repeat([]byte{0x81}, 64)},
			{ValidatorNodeID: "validator-b", Signature: bytes.Repeat([]byte{0x82}, 64)},
			{ValidatorNodeID: "validator-c", Signature: bytes.Repeat([]byte{0x83}, 64)},
		}},
	}, nil
}
func (d *fakeBCOSDriver) Close() error { d.closed = true; return nil }

func TestFISCOBCOSStandardSinkPublishesCompleteRawEvidence(t *testing.T) {
	trust, drivers := fakeBCOSFixture(t)
	sink, err := NewFISCOBCOSStandardSink(FISCOBCOSStandardSinkConfig{
		TrustConfig: trust, Drivers: drivers, Clock: func() time.Time { return time.Unix(10, 0) },
	})
	if err != nil {
		t.Fatal(err)
	}
	sth := testSTH(testScheduleKey(fiscobcos.SinkName), 8, 0x18)
	result, err := sink.Publish(context.Background(), sth)
	if err != nil {
		t.Fatal(err)
	}
	if result.SinkName != fiscobcos.SinkName || result.TreeSize != sth.TreeSize || result.PublishedAtUnixN == 0 {
		t.Fatalf("result=%+v", result)
	}
	if err := fiscobcos.ValidateProofAgainstTrustConfig(sth, result, trust); err != nil {
		t.Fatalf("raw evidence container binding: %v", err)
	}
	proof, err := fiscobcos.UnmarshalProof(result.Proof)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.TransactionAttempts) != 1 || len(proof.Receipt.TransactionProof) != 0 || len(proof.Finality.Signatures) != 3 {
		t.Fatalf("proof=%+v", proof)
	}
}

func TestFISCOBCOSStandardSinkFailsClosedOnEndpointDisagreement(t *testing.T) {
	trust, drivers := fakeBCOSFixture(t)
	drivers[1].(*fakeBCOSDriver).probe.Height++
	sink, err := NewFISCOBCOSStandardSink(FISCOBCOSStandardSinkConfig{TrustConfig: trust, Drivers: drivers})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sink.Publish(context.Background(), testSTH(testScheduleKey(fiscobcos.SinkName), 8, 0x18))
	if !errors.Is(err, ErrPermanent) || !errors.Is(err, fiscobcos.ErrEndpointDisagreement) {
		t.Fatalf("height disagreement error=%v", err)
	}
	state := drivers[0].(*fakeBCOSDriver).state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.submitCalls != 0 {
		t.Fatalf("side effect occurred before quorum probe: calls=%d", state.submitCalls)
	}
}

func TestFISCOBCOSStandardSinkRejectsReceiptOnlyResponse(t *testing.T) {
	trust, drivers := fakeBCOSFixture(t)
	state := drivers[0].(*fakeBCOSDriver).state
	driver := drivers[0].(*fakeBCOSDriver)
	originalSubmit := driver.state
	_ = originalSubmit
	sink, err := NewFISCOBCOSStandardSink(FISCOBCOSStandardSinkConfig{TrustConfig: trust, Drivers: drivers})
	if err != nil {
		t.Fatal(err)
	}
	// The fake produces proof fields during Submit. Clear them after the side
	// effect by wrapping the shared state through a hook-free first call.
	_, _ = driver.SubmitAnchor(context.Background(), fakeSubmitRequest(t, testSTH(testScheduleKey(fiscobcos.SinkName), 7, 0x17)))
	state.mu.Lock()
	state.receipt.Evidence.TransactionProof = nil
	state.receipt.Evidence.ReceiptProof = nil
	state.mu.Unlock()
	// A new Publish replaces the fixture receipt. Force incomplete evidence
	// through a dedicated driver wrapper.
	drivers[0] = &receiptOnlyDriver{fakeBCOSDriver: driver}
	sink, err = NewFISCOBCOSStandardSink(FISCOBCOSStandardSinkConfig{TrustConfig: trust, Drivers: drivers})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sink.Publish(context.Background(), testSTH(testScheduleKey(fiscobcos.SinkName), 8, 0x18))
	if !errors.Is(err, ErrPermanent) || !errors.Is(err, fiscobcos.ErrIncompleteChainEvidence) {
		t.Fatalf("receipt-only error=%v", err)
	}
}

type receiptOnlyDriver struct{ *fakeBCOSDriver }

func (d *receiptOnlyDriver) GetReceiptWithProof(ctx context.Context, hash []byte) (fiscobcos.ReceiptWithProof, error) {
	receipt, err := d.fakeBCOSDriver.GetReceiptWithProof(ctx, hash)
	receipt.Evidence.TransactionProof = nil
	receipt.Evidence.ReceiptProof = nil
	return receipt, err
}

func TestFISCOBCOSServiceRestartKeepsImmutableTarget(t *testing.T) {
	trust, drivers := fakeBCOSFixture(t)
	state := drivers[0].(*fakeBCOSDriver).state
	state.failAfterEffectOnce = true
	sink, err := NewFISCOBCOSStandardSink(FISCOBCOSStandardSinkConfig{TrustConfig: trust, Drivers: drivers})
	if err != nil {
		t.Fatal(err)
	}
	store := proofstore.LocalStore{Root: t.TempDir()}
	key := testScheduleKey(fiscobcos.SinkName)
	sth := testSTH(key, 9, 0x19)
	offer(t, store, key, sth, 100, 100)
	now := time.Unix(0, 100)
	first := newTestService(t, store, sink, key, &now, func(config *Config) {
		config.InitialBackoff = time.Nanosecond
		config.MaxBackoff = time.Nanosecond
	})
	first.tick(context.Background())
	schedule, found, err := store.GetSTHAnchorSchedule(context.Background(), key)
	if err != nil || !found || schedule.InFlight == nil || schedule.InFlight.Target.TreeSize != sth.TreeSize {
		t.Fatalf("schedule after ambiguous submission=%+v found=%v err=%v", schedule, found, err)
	}

	now = now.Add(time.Second)
	second := newTestService(t, store, sink, key, &now, nil)
	second.tick(context.Background())
	result, found, err := store.GetSTHAnchorResult(context.Background(), sth.TreeSize)
	if err != nil || !found || result.TreeSize != sth.TreeSize {
		t.Fatalf("result after restart=%+v found=%v err=%v", result, found, err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.submitCalls != 2 {
		t.Fatalf("submit calls=%d, want 2 attempts for one immutable target", state.submitCalls)
	}
}

func fakeBCOSFixture(t *testing.T) (fiscobcos.TrustConfig, []fiscobcos.Driver) {
	t.Helper()
	trust, err := fiscobcos.NewTrustConfig(fiscobcos.CryptoModeStandard)
	if err != nil {
		t.Fatal(err)
	}
	trust.ChainID = "chain0"
	trust.GroupID = "group0"
	trust.GenesisHash = bytes.Repeat([]byte{0x01}, 32)
	trust.TrustedCheckpoint = fiscobcos.BlockCheckpoint{BlockNumber: 400, BlockHash: bytes.Repeat([]byte{0x21}, 32)}
	trust.Contract = fiscobcos.ContractBinding{
		Address: bytes.Repeat([]byte{0x41}, 20), CodeHash: bytes.Repeat([]byte{0x61}, 32),
		ProtocolVersion: "trustdb-anchor-v1",
		EventSignature:  "AnchorPublished(bytes32,bytes32,uint64,bytes32,bytes32,address,uint16)",
	}
	trust.Endpoints = []string{"127.0.0.1:20200", "127.0.0.1:20201"}
	trust.ReadQuorum = 2
	trust.AccountProvider = fiscobcos.AccountProviderConfig{
		Provider: "software", KeyID: "publisher", KeyReference: "publisher.keyref",
		Algorithm: fiscobcos.StandardAccountAlg,
	}
	trust.Certificates = fiscobcos.CertificateConfig{
		TransportMode:               fiscobcos.StandardTransport,
		TrustedCAReferences:         []string{"ca.crt"},
		TrustedCACertificateHashes:  [][]byte{bytes.Repeat([]byte{0xa1}, 32)},
		ClientSigningCertificateRef: "sdk.crt", ClientSigningKeyRef: "sdk.key",
	}
	for _, id := range []string{"validator-a", "validator-b", "validator-c", "validator-d"} {
		publicKey := append([]byte{0x04}, bytes.Repeat([]byte{byte(len(id))}, 64)...)
		trust.Validators = append(trust.Validators, fiscobcos.ValidatorDescriptor{
			NodeID: id, Algorithm: fiscobcos.StandardAccountAlg,
			PublicKeyEncoding: fiscobcos.StandardKeyEncoding, PublicKey: publicKey,
		})
	}
	probe := fiscobcos.ChainProbe{
		SDKVersion: fiscobcos.StandardSDKVersion, CryptoMode: fiscobcos.CryptoModeStandard,
		ChainID: trust.ChainID, GroupID: trust.GroupID,
		GenesisHash:    append([]byte(nil), trust.GenesisHash...),
		CheckpointHash: append([]byte(nil), trust.TrustedCheckpoint.BlockHash...),
		Height:         500, ContractCodeHash: append([]byte(nil), trust.Contract.CodeHash...),
	}
	state := &fakeBCOSState{}
	drivers := make([]fiscobcos.Driver, 0, len(trust.Endpoints))
	for _, endpoint := range trust.Endpoints {
		candidate := cloneChainProbe(probe)
		candidate.Endpoint = endpoint
		drivers = append(drivers, &fakeBCOSDriver{endpoint: endpoint, probe: candidate, state: state})
	}
	return trust, drivers
}

func fakeSubmitRequest(t *testing.T, sth model.SignedTreeHead) fiscobcos.SubmitRequest {
	t.Helper()
	payload, err := fiscobcos.NewAnchorPayload(cryptosuite.INTLV1, sth)
	if err != nil {
		t.Fatal(err)
	}
	data, err := fiscobcos.MarshalPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	return fiscobcos.SubmitRequest{Payload: payload, CanonicalPayload: data}
}

func cloneAttempt(in fiscobcos.TransactionAttempt) fiscobcos.TransactionAttempt {
	in.RawCanonicalTransaction = append([]byte(nil), in.RawCanonicalTransaction...)
	in.Signature = append([]byte(nil), in.Signature...)
	in.Sender = append([]byte(nil), in.Sender...)
	in.TransactionHash = append([]byte(nil), in.TransactionHash...)
	return in
}

func cloneReceipt(in fiscobcos.ReceiptWithProof) fiscobcos.ReceiptWithProof {
	in.BlockHash = append([]byte(nil), in.BlockHash...)
	in.Record = cloneAnchorRecord(in.Record)
	in.Evidence.RawCanonicalReceipt = append([]byte(nil), in.Evidence.RawCanonicalReceipt...)
	in.Evidence.ReceiptHash = append([]byte(nil), in.Evidence.ReceiptHash...)
	in.Evidence.TransactionHash = append([]byte(nil), in.Evidence.TransactionHash...)
	in.Evidence.TransactionProof = cloneByteSlicesForTest(in.Evidence.TransactionProof)
	in.Evidence.ReceiptProof = cloneByteSlicesForTest(in.Evidence.ReceiptProof)
	in.Evidence.DecodedAnchorEvent = append([]byte(nil), in.Evidence.DecodedAnchorEvent...)
	return in
}

func cloneByteSlicesForTest(in [][]byte) [][]byte {
	if in == nil {
		return nil
	}
	out := make([][]byte, len(in))
	for i := range in {
		out[i] = append([]byte(nil), in[i]...)
	}
	return out
}

func (s *fakeBCOSState) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("submits=%d", s.submitCalls)
}
