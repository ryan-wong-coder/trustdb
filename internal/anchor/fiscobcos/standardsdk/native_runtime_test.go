//go:build fiscobcos_sdk && cgo

package standardsdk

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowtrust/trustdb/internal/anchor/fiscobcos"
)

const nativeVersionFixture = `FISCO BCOS C SDK Version : 3.6.0
Build Time         : 20240219 16:21:14
Build Type         : Darwin/appleclang/MinSizeRel
Git Branch         : main
Git Commit         : 53240138c396c10cb0e1a2b7b4d5c0cdaa0ac539
`

func TestVerifyNativeRuntimeRequiresExactVersionAndArtifact(t *testing.T) {
	t.Parallel()
	content := []byte("pinned native fixture")
	sum := sha256.Sum256(content)
	pin := nativeArtifactPin{
		name:   "libbcos-c-sdk-test",
		size:   int64(len(content)),
		sha256: hex.EncodeToString(sum[:]),
	}
	path := filepath.Join(t.TempDir(), pin.name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := verifyNativeRuntime(nativeVersionFixture, path, pin)
	if err != nil {
		t.Fatalf("verifyNativeRuntime: %v", err)
	}
	if got != fiscobcos.StandardSDKVersion {
		t.Fatalf("SDK identity = %q, want %q", got, fiscobcos.StandardSDKVersion)
	}

	for _, test := range []struct {
		name    string
		version string
		path    string
		pin     nativeArtifactPin
	}{
		{
			name:    "version",
			version: strings.Replace(nativeVersionFixture, "3.6.0", "3.6.1", 1),
			path:    path,
			pin:     pin,
		},
		{
			name:    "commit",
			version: strings.Replace(nativeVersionFixture, supportedNativeCommit, strings.Repeat("0", 40), 1),
			path:    path,
			pin:     pin,
		},
		{
			name:    "unknown version field",
			version: nativeVersionFixture + "Unexpected : value\n",
			path:    path,
			pin:     pin,
		},
		{
			name:    "basename",
			version: nativeVersionFixture,
			path:    path,
			pin: nativeArtifactPin{
				name: "different-library", size: pin.size, sha256: pin.sha256,
			},
		},
		{
			name:    "size",
			version: nativeVersionFixture,
			path:    path,
			pin: nativeArtifactPin{
				name: pin.name, size: pin.size + 1, sha256: pin.sha256,
			},
		},
		{
			name:    "digest",
			version: nativeVersionFixture,
			path:    path,
			pin: nativeArtifactPin{
				name: pin.name, size: pin.size, sha256: strings.Repeat("0", 64),
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := verifyNativeRuntime(test.version, test.path, test.pin); !errors.Is(err, fiscobcos.ErrUnsupportedSDK) {
				t.Fatalf("mismatched native runtime error = %v, want ErrUnsupportedSDK", err)
			}
		})
	}
}

func TestVerifyNativeArtifactRejectsSymlink(t *testing.T) {
	t.Parallel()
	content := []byte("pinned native fixture")
	sum := sha256.Sum256(content)
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "libbcos-c-sdk-test")
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := verifyNativeArtifact(link, nativeArtifactPin{
		name: "libbcos-c-sdk-test", size: int64(len(content)),
		sha256: hex.EncodeToString(sum[:]),
	}); err == nil {
		t.Fatal("accepted symlinked native artifact")
	}
}

func TestVerifyNativeArtifactSupportsUnicodePath(t *testing.T) {
	t.Parallel()
	content := []byte("pinned native fixture")
	sum := sha256.Sum256(content)
	directory := filepath.Join(t.TempDir(), "国密运行时")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "libbcos-c-sdk-test")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyNativeArtifact(path, nativeArtifactPin{
		name: "libbcos-c-sdk-test", size: int64(len(content)),
		sha256: hex.EncodeToString(sum[:]),
	}); err != nil {
		t.Fatalf("unicode native artifact path rejected: %v", err)
	}
}

func TestObservedNativeRuntimeMatchesProtocolPin(t *testing.T) {
	got, err := observeAndVerifyNativeRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if got != fiscobcos.StandardSDKVersion {
		t.Fatalf("SDK identity = %q, want %q", got, fiscobcos.StandardSDKVersion)
	}
}

func TestTransactionHashTextIsExactAndBounded(t *testing.T) {
	t.Parallel()
	valid := "0x" + strings.Repeat("a5", 32)
	if err := validateTransactionHashText(valid); err != nil {
		t.Fatalf("valid transaction hash rejected: %v", err)
	}
	for _, value := range []string{
		"",
		strings.Repeat("a5", 32),
		"0X" + strings.Repeat("a5", 32),
		"0x" + strings.Repeat("a5", 31),
		"0x" + strings.Repeat("a5", 33),
		"0x" + strings.Repeat("gg", 32),
	} {
		if err := validateTransactionHashText(value); err == nil {
			t.Fatalf("invalid transaction hash %q accepted", value)
		}
	}
}
