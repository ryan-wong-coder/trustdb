package receipt

import (
	"crypto/ed25519"
	"fmt"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/trustcrypto"
)

const (
	acceptedDomain  = "trustdb.accepted-receipt.v1"
	committedDomain = "trustdb.committed-receipt.v1"
)

func SignAccepted(r model.AcceptedReceipt, keyID string, privateKey ed25519.PrivateKey) (model.AcceptedReceipt, error) {
	r.ServerSig = model.Signature{}
	payload, err := cborx.Marshal(r)
	if err != nil {
		return model.AcceptedReceipt{}, err
	}
	sig, err := trustcrypto.SignEd25519(keyID, privateKey, domainInput(acceptedDomain, payload))
	if err != nil {
		return model.AcceptedReceipt{}, err
	}
	r.ServerSig = sig
	return r, nil
}

func VerifyAccepted(r model.AcceptedReceipt, publicKey ed25519.PublicKey) error {
	sig := r.ServerSig
	r.ServerSig = model.Signature{}
	payload, err := cborx.Marshal(r)
	if err != nil {
		return err
	}
	if err := trustcrypto.VerifyEd25519(publicKey, domainInput(acceptedDomain, payload), sig); err != nil {
		return fmt.Errorf("verify accepted receipt: %w", err)
	}
	return nil
}

func SignCommitted(r model.CommittedReceipt, keyID string, privateKey ed25519.PrivateKey) (model.CommittedReceipt, error) {
	r.ServerSig = model.Signature{}
	payload, err := cborx.Marshal(r)
	if err != nil {
		return model.CommittedReceipt{}, err
	}
	sig, err := trustcrypto.SignEd25519(keyID, privateKey, domainInput(committedDomain, payload))
	if err != nil {
		return model.CommittedReceipt{}, err
	}
	r.ServerSig = sig
	return r, nil
}

func VerifyCommitted(r model.CommittedReceipt, publicKey ed25519.PublicKey) error {
	sig := r.ServerSig
	r.ServerSig = model.Signature{}
	payload, err := cborx.Marshal(r)
	if err != nil {
		return err
	}
	if err := trustcrypto.VerifyEd25519(publicKey, domainInput(committedDomain, payload), sig); err != nil {
		return fmt.Errorf("verify committed receipt: %w", err)
	}
	return nil
}

func domainInput(domain string, payload []byte) []byte {
	out := make([]byte, 0, len(domain)+1+len(payload))
	out = append(out, domain...)
	out = append(out, 0)
	out = append(out, payload...)
	return out
}
