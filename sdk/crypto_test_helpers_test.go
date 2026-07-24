package sdk

import (
	"crypto/ed25519"
	"testing"
)

func mustINTLV1Identity(t testing.TB, tenantID, clientID, keyID string, privateKey ed25519.PrivateKey) Identity {
	t.Helper()
	identity, err := NewINTLV1Identity(tenantID, clientID, keyID, privateKey)
	if err != nil {
		t.Fatalf("NewINTLV1Identity: %v", err)
	}
	return identity
}

func mustINTLV1PublicKey(t testing.TB, keyID string, publicKey ed25519.PublicKey) KeyDescriptor {
	t.Helper()
	descriptor, err := NewINTLV1PublicKey(keyID, publicKey)
	if err != nil {
		t.Fatalf("NewINTLV1PublicKey: %v", err)
	}
	return descriptor
}

func mustCNSMV1Identity(t testing.TB, tenantID, clientID, keyID string, privateKey []byte) Identity {
	t.Helper()
	identity, err := NewCNSMV1Identity(tenantID, clientID, keyID, privateKey)
	if err != nil {
		t.Fatalf("NewCNSMV1Identity: %v", err)
	}
	return identity
}

func mustCNSMV1PublicKey(t testing.TB, keyID string, publicKey []byte) KeyDescriptor {
	t.Helper()
	descriptor, err := NewCNSMV1PublicKey(keyID, publicKey)
	if err != nil {
		t.Fatalf("NewCNSMV1PublicKey: %v", err)
	}
	return descriptor
}
