//go:build integration

package tikv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/cborx"
	"github.com/ryan-wong-coder/trustdb/internal/model"
)

func TestTiKVMigrateLegacyKeys(t *testing.T) {
	requireInternalTiKVIntegration(t)
	if os.Getenv("TRUSTDB_TIKV_RUN_LEGACY_MIGRATION_TEST") != "1" {
		t.Skip("set TRUSTDB_TIKV_RUN_LEGACY_MIGRATION_TEST=1 to copy legacy bare TiKV keys")
	}

	ctx := context.Background()
	store := openInternalIntegrationStore(t, internalIntegrationNamespace(t, "migration"))
	defer store.Close()

	legacyKey := bundleKey("legacy-" + internalUniqueTestID(t))
	legacyValue := []byte("legacy-proof-bundle-placeholder")
	if err := store.db.rawSet(legacyKey, legacyValue); err != nil {
		t.Fatalf("seed legacy key: %v", err)
	}
	t.Cleanup(func() { _ = store.db.rawDelete(legacyKey) })

	report, err := store.MigrateLegacyKeys(ctx, MigrationOptions{})
	if err != nil {
		t.Fatalf("MigrateLegacyKeys: %v", err)
	}
	if report.Scanned == 0 || report.Copied == 0 {
		t.Fatalf("migration report = %+v, want at least one copied key", report)
	}
	got, _, err := store.db.rawGet(store.db.physicalKey(legacyKey))
	if err != nil {
		t.Fatalf("read migrated key: %v", err)
	}
	if !bytes.Equal(got, legacyValue) {
		t.Fatalf("migrated value = %q, want %q", got, legacyValue)
	}
}

func TestTiKVMigrateLegacyCheckpointRoundTrip(t *testing.T) {
	requireInternalTiKVIntegration(t)
	if os.Getenv("TRUSTDB_TIKV_RUN_LEGACY_MIGRATION_TEST") != "1" {
		t.Skip("set TRUSTDB_TIKV_RUN_LEGACY_MIGRATION_TEST=1 to copy legacy bare TiKV keys")
	}

	ctx := context.Background()
	store := openInternalIntegrationStore(t, internalIntegrationNamespace(t, "migration-checkpoint"))
	defer store.Close()

	want := model.WALCheckpoint{
		SchemaVersion:   model.SchemaWALCheckpoint,
		SegmentID:       11,
		LastSequence:    99,
		BatchID:         "legacy-checkpoint",
		RecordedAtUnixN: time.Now().UTC().UnixNano(),
	}
	data, err := cborx.Marshal(want)
	if err != nil {
		t.Fatalf("marshal checkpoint: %v", err)
	}
	if err := store.db.rawSet([]byte(checkpointKey), data); err != nil {
		t.Fatalf("seed legacy checkpoint: %v", err)
	}
	t.Cleanup(func() { _ = store.db.rawDelete([]byte(checkpointKey)) })

	report, err := store.MigrateLegacyKeys(ctx, MigrationOptions{})
	if err != nil {
		t.Fatalf("MigrateLegacyKeys: %v", err)
	}
	if report.Copied == 0 {
		t.Fatalf("migration report = %+v, want copied checkpoint", report)
	}
	got, ok, err := store.GetCheckpoint(ctx)
	if err != nil || !ok {
		t.Fatalf("GetCheckpoint after migration ok=%v err=%v", ok, err)
	}
	if got.SegmentID != want.SegmentID || got.LastSequence != want.LastSequence || got.BatchID != want.BatchID {
		t.Fatalf("migrated checkpoint = %+v, want %+v", got, want)
	}
}

func requireInternalTiKVIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TRUSTDB_TIKV_PD_ENDPOINTS") == "" {
		t.Skip("set TRUSTDB_TIKV_PD_ENDPOINTS to run TiKV integration tests")
	}
}

func openInternalIntegrationStore(t *testing.T, namespace string) *Store {
	t.Helper()
	store, err := OpenWithOptions(Options{
		PDAddressText:    os.Getenv("TRUSTDB_TIKV_PD_ENDPOINTS"),
		Keyspace:         os.Getenv("TRUSTDB_TIKV_KEYSPACE"),
		Namespace:        namespace,
		RecordIndexMode:  RecordIndexModeFull,
		ArtifactSyncMode: ArtifactSyncModeChunk,
	})
	if err != nil {
		t.Fatalf("open TiKV store: %v", err)
	}
	return store
}

func internalIntegrationNamespace(t *testing.T, prefix string) string {
	t.Helper()
	return "integration/" + prefix + "/" + internalUniqueTestID(t)
}

func internalUniqueTestID(t *testing.T) string {
	t.Helper()
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	return fmt.Sprintf("%s/%d", re.ReplaceAllString(t.Name(), "_"), time.Now().UTC().UnixNano())
}
