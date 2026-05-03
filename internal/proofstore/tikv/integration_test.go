//go:build integration

package tikv_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/proofstore"
	"github.com/ryan-wong-coder/trustdb/internal/proofstore/proofstoretest"
	tikvstore "github.com/ryan-wong-coder/trustdb/internal/proofstore/tikv"
)

func TestTiKVConformance(t *testing.T) {
	requireTiKVIntegration(t)

	proofstoretest.RunConformance(t, func(t *testing.T) (proofstore.Store, func()) {
		store := openIntegrationStore(t, integrationNamespace(t, "conformance"))
		return store, func() { _ = store.Close() }
	})
}

func TestTiKVSharedNamespaceAcrossStores(t *testing.T) {
	requireTiKVIntegration(t)

	ctx := context.Background()
	namespace := integrationNamespace(t, "shared")
	nodeA := openIntegrationStore(t, namespace)
	defer nodeA.Close()
	nodeB := openIntegrationStore(t, namespace)
	defer nodeB.Close()

	want := model.WALCheckpoint{
		SchemaVersion:   model.SchemaWALCheckpoint,
		SegmentID:       7,
		LastSequence:    42,
		LastOffset:      4096,
		BatchID:         "batch-shared",
		RecordedAtUnixN: time.Now().UTC().UnixNano(),
	}
	if err := nodeA.PutCheckpoint(ctx, want); err != nil {
		t.Fatalf("nodeA PutCheckpoint: %v", err)
	}
	got, ok, err := nodeB.GetCheckpoint(ctx)
	if err != nil || !ok {
		t.Fatalf("nodeB GetCheckpoint ok=%v err=%v", ok, err)
	}
	if got.SegmentID != want.SegmentID || got.LastSequence != want.LastSequence || got.BatchID != want.BatchID {
		t.Fatalf("shared checkpoint = %+v, want %+v", got, want)
	}
}

func TestTiKVNamespaceIsolationAcrossStores(t *testing.T) {
	requireTiKVIntegration(t)

	ctx := context.Background()
	nodeA := openIntegrationStore(t, integrationNamespace(t, "isolation-a"))
	defer nodeA.Close()
	nodeB := openIntegrationStore(t, integrationNamespace(t, "isolation-b"))
	defer nodeB.Close()

	if err := nodeA.PutCheckpoint(ctx, model.WALCheckpoint{
		SchemaVersion:   model.SchemaWALCheckpoint,
		SegmentID:       1,
		LastSequence:    1,
		RecordedAtUnixN: time.Now().UTC().UnixNano(),
	}); err != nil {
		t.Fatalf("nodeA PutCheckpoint: %v", err)
	}
	if _, ok, err := nodeB.GetCheckpoint(ctx); err != nil || ok {
		t.Fatalf("nodeB GetCheckpoint ok=%v err=%v, want missing without error", ok, err)
	}
}

func requireTiKVIntegration(t *testing.T) {
	t.Helper()
	if strings := os.Getenv("TRUSTDB_TIKV_PD_ENDPOINTS"); strings == "" {
		t.Skip("set TRUSTDB_TIKV_PD_ENDPOINTS to run TiKV integration tests")
	}
}

func openIntegrationStore(t *testing.T, namespace string) *tikvstore.Store {
	t.Helper()
	store, err := tikvstore.OpenWithOptions(tikvstore.Options{
		PDAddressText:    os.Getenv("TRUSTDB_TIKV_PD_ENDPOINTS"),
		Keyspace:         os.Getenv("TRUSTDB_TIKV_KEYSPACE"),
		Namespace:        namespace,
		RecordIndexMode:  tikvstore.RecordIndexModeFull,
		ArtifactSyncMode: tikvstore.ArtifactSyncModeChunk,
	})
	if err != nil {
		t.Fatalf("open TiKV store: %v", err)
	}
	return store
}

func integrationNamespace(t *testing.T, prefix string) string {
	t.Helper()
	return "integration/" + prefix + "/" + uniqueTestID(t)
}

func uniqueTestID(t *testing.T) string {
	t.Helper()
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	return fmt.Sprintf("%s/%d", re.ReplaceAllString(t.Name(), "_"), time.Now().UTC().UnixNano())
}
