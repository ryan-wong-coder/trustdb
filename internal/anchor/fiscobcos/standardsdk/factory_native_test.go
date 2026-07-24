//go:build fiscobcos_sdk && cgo

package standardsdk

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/FISCO-BCOS/go-sdk/v3/types"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"

	"github.com/wowtrust/trustdb/internal/anchor/fiscobcos"
)

func TestValidateSignerSignatureRequiresConfiguredPublicKey(t *testing.T) {
	t.Parallel()
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("transaction"))
	signature, err := ethcrypto.Sign(digest[:], key)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := ethcrypto.FromECDSAPub(&key.PublicKey)
	if err := validateSignerSignature(digest[:], signature, publicKey); err != nil {
		t.Fatal(err)
	}
	other, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSignerSignature(digest[:], signature, ethcrypto.FromECDSAPub(&other.PublicKey)); err == nil {
		t.Fatal("accepted signature from a different account")
	}
}

func TestSubmittedReceiptBindingAndBoundedStatusArePanicFree(t *testing.T) {
	t.Parallel()
	digest := bytes.Repeat([]byte{0x11}, 32)
	sender := bytes.Repeat([]byte{0x22}, 20)
	contract := bytes.Repeat([]byte{0x33}, 20)
	input := []byte{1, 2, 3, 4}
	receipt := &types.Receipt{
		Status:          types.Success,
		TransactionHash: "0x" + hex.EncodeToString(digest),
		From:            "0x" + hex.EncodeToString(sender),
		To:              "0x" + hex.EncodeToString(contract),
		Input:           "0x" + hex.EncodeToString(input),
	}
	if err := validateSubmittedReceipt(receipt, digest, sender, contract, input); err != nil {
		t.Fatal(err)
	}
	receipt.TransactionHash = "0x" + strings.Repeat("44", 32)
	if err := validateSubmittedReceipt(receipt, digest, sender, contract, input); err == nil {
		t.Fatal("accepted mismatched transaction hash")
	}
	for _, status := range []int{types.Success, types.BlockLimitCheckFail, -1, int(^uint(0) >> 1)} {
		if got := boundedReceiptStatus(status); got == "" || len(got) > 64 {
			t.Fatalf("boundedReceiptStatus(%d)=%q", status, got)
		}
	}
}

func TestReceiptTransactionIdentityChecksEveryField(t *testing.T) {
	t.Parallel()
	hash := common.BytesToHash(bytes.Repeat([]byte{0x51}, 32))
	sender := bytes.Repeat([]byte{0x52}, 20)
	contract := bytes.Repeat([]byte{0x53}, 20)
	input := []byte{0x54, 0x55}
	receipt := &types.Receipt{
		TransactionHash: hash.Hex(),
		From:            "0x" + hex.EncodeToString(sender),
		To:              "0x" + hex.EncodeToString(contract),
		Input:           "0x" + hex.EncodeToString(input),
	}
	transaction := &types.TransactionDetail{
		Hash: hash.Hex(), From: receipt.From, To: receipt.To, Input: receipt.Input,
	}
	if err := validateReceiptTransactionIdentity(receipt, transaction, hash, sender, contract); err != nil {
		t.Fatal(err)
	}
	transaction.Input = "0x00"
	if err := validateReceiptTransactionIdentity(receipt, transaction, hash, sender, contract); err == nil {
		t.Fatal("accepted receipt/transaction input mismatch")
	}
}

func TestLocalReferencesAreAbsoluteRegularAndPrivate(t *testing.T) {
	t.Parallel()
	if _, err := localPath("relative/key.pem"); err == nil {
		t.Fatal("accepted a relative local reference")
	}
	root := t.TempDir()
	caPath := filepath.Join(root, "ca.crt")
	certPath := filepath.Join(root, "sdk.crt")
	keyPath := filepath.Join(root, "sdk.key")
	for path, mode := range map[string]os.FileMode{caPath: 0o644, certPath: 0o644, keyPath: 0o600} {
		if err := os.WriteFile(path, []byte("not-empty"), mode); err != nil {
			t.Fatal(err)
		}
	}
	caDigest := sha256.Sum256([]byte("not-empty"))
	config := fiscobcos.TrustConfig{
		AccountProvider: fiscobcos.AccountProviderConfig{
			Provider: "sdf", KeyReference: "sdf://slot/7",
		},
		Certificates: fiscobcos.CertificateConfig{
			TrustedCAReferences:         []string{caPath},
			TrustedCACertificateHashes:  [][]byte{caDigest[:]},
			ClientSigningCertificateRef: certPath,
			ClientSigningKeyRef:         keyPath,
		},
	}
	if err := verifyCertificateReferences(config, false); err != nil {
		t.Fatalf("injected signer should not read opaque account key reference: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(keyPath, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := verifyCertificateReferences(config, false); err == nil {
			t.Fatal("accepted group-readable TLS private key")
		}
		if err := os.Chmod(keyPath, 0o600); err != nil {
			t.Fatal(err)
		}
		linkPath := filepath.Join(root, "key-link")
		if err := os.Symlink(keyPath, linkPath); err != nil {
			t.Fatal(err)
		}
		if _, err := readBoundedRegularFile(linkPath, true); err == nil {
			t.Fatal("accepted symlinked private key")
		}
	}
}
