//go:build sdf && cgo && (linux || darwin)

package sdfsigner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wowtrust/trustdb/sdk/signerplugin"
)

func TestNativeAdapterABIContract(t *testing.T) {
	library := buildFakeNativeAdapter(t)
	backend, err := OpenNativeBackend(library, []byte("normal"))
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	device, err := backend.Discover(context.Background(), "sdf-production")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := device.Identity(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if identity.DeviceID != "sdf-production" || identity.AdapterID != "trustdb.fake-sdf" {
		t.Fatalf("identity = %+v", identity)
	}
	capabilities, err := device.Capabilities(context.Background())
	if err != nil || capabilities != AllCapabilities {
		t.Fatalf("capabilities = %#x, %v", capabilities, err)
	}
	session, err := device.OpenSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	credential := []byte("846295")
	publicKey, err := session.PublicKey(context.Background(), 7, credential)
	if err != nil || len(publicKey) != 65 {
		t.Fatalf("PublicKey() = %x, %v", publicKey, err)
	}
	digest := make([]byte, 32)
	for index := range digest {
		digest[index] = byte(index)
	}
	signature, err := session.SignSM2Digest(context.Background(), 7, credential, digest)
	if err != nil || len(signature) != 64 ||
		string(signature[:32]) != string(digest) || string(signature[32:]) != string(digest) {
		t.Fatalf("SignSM2Digest() = %x, %v", signature, err)
	}
	random, err := session.Random(context.Background(), 64)
	if err != nil || len(random) != 64 || random[1] != 0xa4 {
		t.Fatalf("Random() = %x, %v", random, err)
	}
	wrapped, generated, err := session.GenerateSM4KeyWithKEK(context.Background(), 11, credential)
	if err != nil || len(wrapped) != 17 || generated.value == 0 {
		t.Fatalf("GenerateSM4KeyWithKEK() = %x, %#v, %v", wrapped, generated, err)
	}
	iv := []byte("0123456789abcdef")
	plaintext := []byte("block-0000000001")
	ciphertext, err := session.EncryptSM4CBC(context.Background(), generated, iv, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := session.DecryptSM4CBC(context.Background(), generated, iv, ciphertext)
	if err != nil || string(roundTrip) != string(plaintext) {
		t.Fatalf("DecryptSM4CBC() = %q, %v", roundTrip, err)
	}
	if err := session.DestroySessionKey(context.Background(), generated); err != nil {
		t.Fatal(err)
	}
	imported, err := session.ImportSM4KeyWithKEK(context.Background(), 11, credential, wrapped)
	if err != nil || imported.value == 0 {
		t.Fatalf("ImportSM4KeyWithKEK() = %#v, %v", imported, err)
	}
	mac, err := session.CalculateSM4MAC(context.Background(), imported, iv, plaintext)
	if err != nil || len(mac) != SM4BlockBytes {
		t.Fatalf("CalculateSM4MAC() = %x, %v", mac, err)
	}
	if err := session.DestroySessionKey(context.Background(), imported); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeAdapterRejectsMalformedIdentityAndSanitizesStatus(t *testing.T) {
	library := buildFakeNativeAdapter(t)
	t.Run("identity", func(t *testing.T) {
		backend, err := OpenNativeBackend(library, []byte("bad-identity"))
		if err != nil {
			t.Fatal(err)
		}
		defer backend.Close()
		device, err := backend.Discover(context.Background(), "sdf-production")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := device.Identity(context.Background()); err == nil {
			t.Fatal("Identity() accepted a non-terminated native field")
		}
	})
	t.Run("status", func(t *testing.T) {
		backend, err := OpenNativeBackend(library, []byte("health=busy"))
		if err != nil {
			t.Fatal(err)
		}
		defer backend.Close()
		device, _ := backend.Discover(context.Background(), "sdf-production")
		session, _ := device.OpenSession(context.Background())
		defer session.Close()
		err = session.Health(context.Background())
		var fault *Fault
		if !errors.As(err, &fault) || fault.class != faultBusy {
			t.Fatalf("Health() error = %v", err)
		}
		if strings.Contains(err.Error(), "busy") || strings.Contains(err.Error(), "health") {
			t.Fatalf("native diagnostic leaked: %v", err)
		}
	})
}

func TestOpenNativeBackendRejectsNonAdapterLibraryWithoutPathLeak(t *testing.T) {
	path := "/definitely/not/a/customer-secret-adapter.so"
	_, err := OpenNativeBackend(path, []byte("normal"))
	requireProviderCode(t, providerError(err), signerplugin.ErrorUnavailable)
	if strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "customer-secret") {
		t.Fatalf("loader error leaked path: %v", err)
	}
}

func buildFakeNativeAdapter(t *testing.T) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "libtrustdb-fake-sdf.so")
	args := []string{
		"-shared", "-fPIC",
		"-I", filepath.Join("..", "..", "sdk", "sdfadapter"),
		"-o", output,
		filepath.Join("testdata", "fake_adapter.c"),
	}
	if runtime.GOOS == "darwin" {
		args[0] = "-dynamiclib"
	}
	command := exec.Command("cc", args...)
	if outputBytes, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile fake adapter: %v\n%s", err, outputBytes)
	}
	return output
}
