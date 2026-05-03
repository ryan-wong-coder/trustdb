package config

import (
	"strings"
	"testing"
)

func TestDefaultConfigIsValid(t *testing.T) {
	t.Parallel()

	if err := Default().Validate(); err != nil {
		t.Fatalf("default config is invalid: %v", err)
	}
}

func TestDefaultYAMLIsStructured(t *testing.T) {
	t.Parallel()

	for _, section := range []string{"paths:", "identity:", "server:", "registry:", "batch:", "proofstore:", "log:", "keys:"} {
		if !strings.Contains(DefaultYAML, section) {
			t.Fatalf("default yaml missing section %q", section)
		}
	}
	if Default().Batch.ProofMode != "inline" {
		t.Fatalf("default batch.proof_mode = %q, want inline", Default().Batch.ProofMode)
	}
	if Default().Proofstore.ArtifactSyncMode != "chunk" {
		t.Fatalf("default proofstore.artifact_sync_mode = %q, want chunk", Default().Proofstore.ArtifactSyncMode)
	}
	if Default().Proofstore.RecordIndexMode != "full" {
		t.Fatalf("default proofstore.record_index_mode = %q, want full", Default().Proofstore.RecordIndexMode)
	}
	if Default().GlobalLog.LogID != "trustdb-global-log" {
		t.Fatalf("default global_log.log_id = %q, want trustdb-global-log", Default().GlobalLog.LogID)
	}
	if Default().Proofstore.TiKVPDAddresses != nil {
		t.Fatalf("default proofstore.tikv_pd_endpoints = %#v, want nil", Default().Proofstore.TiKVPDAddresses)
	}
	if Default().Proofstore.TiKVNamespace != "default" {
		t.Fatalf("default proofstore.tikv_namespace = %q, want default", Default().Proofstore.TiKVNamespace)
	}
}

func TestValidateRejectsInvalidLogConfig(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Log.Format = "console"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate rejected console log format: %v", err)
	}

	cfg = Default()
	cfg.Log.Level = "trace"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid log level")
	}

	cfg = Default()
	cfg.Log.Format = "pretty"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid log format")
	}

	cfg = Default()
	cfg.Log.Output = "syslog"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid log output")
	}

	cfg = Default()
	cfg.Log.Output = "file"
	cfg.Log.File.Path = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted file output without file path")
	}

	cfg = Default()
	cfg.Log.File.MaxSizeMB = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid log max size")
	}

	cfg = Default()
	cfg.Log.Async.BufferSize = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid async log buffer size")
	}
}

func TestValidateRejectsInvalidBatchConfig(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Batch.QueueSize = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid batch queue size")
	}

	cfg = Default()
	cfg.Batch.MaxRecords = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid batch max records")
	}

	cfg = Default()
	cfg.Batch.MaxDelay = "soon"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid batch max delay")
	}

	cfg = Default()
	cfg.Batch.ProofMode = "eventually"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid batch proof mode")
	}
}

func TestValidateRejectsInvalidProofstoreConfig(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Proofstore.ArtifactSyncMode = "sometimes"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid proofstore artifact sync mode")
	}

	cfg = Default()
	cfg.Proofstore.RecordIndexMode = "none"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted invalid proofstore record index mode")
	}
}

func TestRedactedHidesKeyPaths(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Keys.ClientPrivate = "client.key"
	cfg.Keys.ServerPublic = "server.pub"
	redacted := cfg.Redacted()
	if redacted.Keys.ClientPrivate != "<redacted>" {
		t.Fatalf("client private = %q", redacted.Keys.ClientPrivate)
	}
	if redacted.Keys.ServerPublic != "<redacted>" {
		t.Fatalf("server public = %q", redacted.Keys.ServerPublic)
	}
	if redacted.Paths.DataDir != cfg.Paths.DataDir {
		t.Fatalf("paths should not be redacted")
	}
}
