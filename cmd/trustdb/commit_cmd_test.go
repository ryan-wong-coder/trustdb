package main

import (
	"crypto/ed25519"
	"path/filepath"
	"testing"
)

// TestResolveClientKeysPrefersExplicitPubKey guards the bug where a non-empty
// default --key-registry (".trustdb/keys.tdkeys") silently overrode an
// explicitly-supplied --client-public-key. When the operator did NOT pass
// --key-registry on the command line, we want the pub key to win even if
// viper handed us a registry path from the defaults.
func TestResolveClientKeysPrefersExplicitPubKey(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	pubPath := filepath.Join(tmp, "client.pub")
	if err := writeKey(pubPath, pub); err != nil {
		t.Fatalf("writeKey() error = %v", err)
	}

	// Registry path points at a location that does not exist on disk — if
	// the old code path ran we'd get a keystore open error instead of the
	// single-key trust anchor we want.
	bogusRegistry := filepath.Join(tmp, "does-not-exist.tdkeys")

	got, resolver, err := resolveClientKeys(pubPath, bogusRegistry, "", false)
	if err != nil {
		t.Fatalf("resolveClientKeys() error = %v", err)
	}
	if resolver != nil {
		t.Fatalf("resolveClientKeys() resolver = %v, want nil (pub-key branch)", resolver)
	}
	if len(got) != ed25519.PublicKeySize || !got.Equal(pub) {
		t.Fatalf("resolveClientKeys() pub key mismatch: %x vs %x", got, pub)
	}
}

// TestResolveClientKeysExplicitRegistryWins makes sure the operator can still
// force the registry backend even when a pub-key is also available, as long
// as they flipped --key-registry themselves.
func TestResolveClientKeysExplicitRegistryWins(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	pubPath := filepath.Join(tmp, "client.pub")
	if err := writeKey(pubPath, pub); err != nil {
		t.Fatalf("writeKey() error = %v", err)
	}
	registryPath := filepath.Join(tmp, "keys.tdkeys")

	gotPub, resolver, err := resolveClientKeys(pubPath, registryPath, "", true)
	if err != nil {
		t.Fatalf("resolveClientKeys() error = %v", err)
	}
	if gotPub != nil {
		t.Fatalf("resolveClientKeys() pub = %x, want nil (registry branch)", gotPub)
	}
	if resolver == nil {
		t.Fatalf("resolveClientKeys() resolver = nil, want registry-backed resolver")
	}
}

// TestResolveClientKeysRegistryFallback ensures deployments that rely on a
// default (non-explicit) registry and do NOT supply a pub key still open the
// registry as before — no regression for registry-first setups.
func TestResolveClientKeysRegistryFallback(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	registryPath := filepath.Join(tmp, "keys.tdkeys")

	gotPub, resolver, err := resolveClientKeys("", registryPath, "", false)
	if err != nil {
		t.Fatalf("resolveClientKeys() error = %v", err)
	}
	if gotPub != nil {
		t.Fatalf("resolveClientKeys() pub = %x, want nil", gotPub)
	}
	if resolver == nil {
		t.Fatalf("resolveClientKeys() resolver = nil, want registry-backed resolver")
	}
}
