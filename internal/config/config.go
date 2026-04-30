package config

import (
	"fmt"
	"strings"
	"time"
)

const DefaultYAML = `# TrustDB local client configuration.
paths:
  data_dir: ".trustdb"
  key_registry: ".trustdb/keys.tdkeys"
  wal: ".trustdb/trustdb.wal"
  object_dir: ".trustdb/objects"
  proof_dir: ".trustdb/proofs"

metastore: "pebble"
metastore_path: ".trustdb/proofs/pebble"

proofstore:
  artifact_sync_mode: "chunk"
  record_index_mode: "full"

wal:
  fsync_mode: "group"
  group_commit_interval: "10ms"

identity:
  tenant: "default"
  client: ""
  key_id: ""

server:
  listen: "127.0.0.1:8080"
  grpc_listen: ""
  id: "local-server"
  key_id: "server-key"
  queue_size: 1024
  workers: 4
  read_timeout: "10s"
  write_timeout: "10s"
  shutdown_timeout: "10s"

registry:
  key_id: "registry-key"

batch:
  queue_size: 1024
  max_records: 1024
  max_delay: "500ms"
  proof_mode: "inline"

global_log:
  enabled: true

anchor:
  scope: "global"
  max_delay: "5m"

history:
  tile_size: 256
  hot_window_leaves: 65536

backup:
  compression: "gzip"

log:
  level: "warn"
  format: "json"
  output: "stderr"
  file:
    path: ".trustdb/logs/trustdb.log"
    max_size_mb: 256
    max_backups: 16
    max_age_days: 30
    compress: true
  async:
    enabled: false
    buffer_size: 8192
    drop_on_full: false

keys:
  client_private: ""
  client_public: ""
  server_private: ""
  server_public: ""
  registry_private: ""
  registry_public: ""
`

type Config struct {
	Paths      Paths      `mapstructure:"paths" json:"paths"`
	Identity   Identity   `mapstructure:"identity" json:"identity"`
	Server     Server     `mapstructure:"server" json:"server"`
	Registry   Registry   `mapstructure:"registry" json:"registry"`
	Batch      Batch      `mapstructure:"batch" json:"batch"`
	GlobalLog  GlobalLog  `mapstructure:"global_log" json:"global_log"`
	Anchor     Anchor     `mapstructure:"anchor" json:"anchor"`
	History    History    `mapstructure:"history" json:"history"`
	Backup     Backup     `mapstructure:"backup" json:"backup"`
	Proofstore Proofstore `mapstructure:"proofstore" json:"proofstore"`
	Log        Log        `mapstructure:"log" json:"log"`
	Keys       Keys       `mapstructure:"keys" json:"keys"`
}

type Paths struct {
	DataDir     string `mapstructure:"data_dir" json:"data_dir"`
	KeyRegistry string `mapstructure:"key_registry" json:"key_registry"`
	WAL         string `mapstructure:"wal" json:"wal"`
	ObjectDir   string `mapstructure:"object_dir" json:"object_dir"`
	ProofDir    string `mapstructure:"proof_dir" json:"proof_dir"`
}

type Identity struct {
	Tenant string `mapstructure:"tenant" json:"tenant"`
	Client string `mapstructure:"client" json:"client"`
	KeyID  string `mapstructure:"key_id" json:"key_id"`
}

type Server struct {
	Listen          string `mapstructure:"listen" json:"listen"`
	GRPCListen      string `mapstructure:"grpc_listen" json:"grpc_listen"`
	ID              string `mapstructure:"id" json:"id"`
	KeyID           string `mapstructure:"key_id" json:"key_id"`
	QueueSize       int    `mapstructure:"queue_size" json:"queue_size"`
	Workers         int    `mapstructure:"workers" json:"workers"`
	ReadTimeout     string `mapstructure:"read_timeout" json:"read_timeout"`
	WriteTimeout    string `mapstructure:"write_timeout" json:"write_timeout"`
	ShutdownTimeout string `mapstructure:"shutdown_timeout" json:"shutdown_timeout"`
}

type Registry struct {
	KeyID string `mapstructure:"key_id" json:"key_id"`
}

type Batch struct {
	QueueSize  int    `mapstructure:"queue_size" json:"queue_size"`
	MaxRecords int    `mapstructure:"max_records" json:"max_records"`
	MaxDelay   string `mapstructure:"max_delay" json:"max_delay"`
	ProofMode  string `mapstructure:"proof_mode" json:"proof_mode"`
}

type GlobalLog struct {
	Enabled bool `mapstructure:"enabled" json:"enabled"`
}

type Anchor struct {
	Scope    string `mapstructure:"scope" json:"scope"`
	MaxDelay string `mapstructure:"max_delay" json:"max_delay"`
}

type History struct {
	TileSize        uint64 `mapstructure:"tile_size" json:"tile_size"`
	HotWindowLeaves uint64 `mapstructure:"hot_window_leaves" json:"hot_window_leaves"`
}

type Backup struct {
	Compression string `mapstructure:"compression" json:"compression"`
}

type Proofstore struct {
	ArtifactSyncMode string `mapstructure:"artifact_sync_mode" json:"artifact_sync_mode"`
	RecordIndexMode  string `mapstructure:"record_index_mode" json:"record_index_mode"`
}

type Log struct {
	Level  string   `mapstructure:"level" json:"level"`
	Format string   `mapstructure:"format" json:"format"`
	Output string   `mapstructure:"output" json:"output"`
	File   LogFile  `mapstructure:"file" json:"file"`
	Async  LogAsync `mapstructure:"async" json:"async"`
}

type LogFile struct {
	Path       string `mapstructure:"path" json:"path"`
	MaxSizeMB  int    `mapstructure:"max_size_mb" json:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups" json:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days" json:"max_age_days"`
	Compress   bool   `mapstructure:"compress" json:"compress"`
}

type LogAsync struct {
	Enabled    bool `mapstructure:"enabled" json:"enabled"`
	BufferSize int  `mapstructure:"buffer_size" json:"buffer_size"`
	DropOnFull bool `mapstructure:"drop_on_full" json:"drop_on_full"`
}

type Keys struct {
	ClientPrivate   string `mapstructure:"client_private" json:"client_private"`
	ClientPublic    string `mapstructure:"client_public" json:"client_public"`
	ServerPrivate   string `mapstructure:"server_private" json:"server_private"`
	ServerPublic    string `mapstructure:"server_public" json:"server_public"`
	RegistryPrivate string `mapstructure:"registry_private" json:"registry_private"`
	RegistryPublic  string `mapstructure:"registry_public" json:"registry_public"`
}

func Default() Config {
	return Config{
		Paths: Paths{
			DataDir:     ".trustdb",
			KeyRegistry: ".trustdb/keys.tdkeys",
			WAL:         ".trustdb/trustdb.wal",
			ObjectDir:   ".trustdb/objects",
			ProofDir:    ".trustdb/proofs",
		},
		Identity: Identity{
			Tenant: "default",
		},
		Server: Server{
			Listen:          "127.0.0.1:8080",
			ID:              "local-server",
			KeyID:           "server-key",
			QueueSize:       1024,
			Workers:         4,
			ReadTimeout:     "10s",
			WriteTimeout:    "10s",
			ShutdownTimeout: "10s",
		},
		Registry: Registry{
			KeyID: "registry-key",
		},
		Batch: Batch{
			QueueSize:  1024,
			MaxRecords: 1024,
			MaxDelay:   "500ms",
			ProofMode:  "inline",
		},
		GlobalLog: GlobalLog{
			Enabled: true,
		},
		Anchor: Anchor{
			Scope:    "global",
			MaxDelay: "5m",
		},
		History: History{
			TileSize:        256,
			HotWindowLeaves: 65536,
		},
		Backup: Backup{
			Compression: "gzip",
		},
		Proofstore: Proofstore{
			ArtifactSyncMode: "chunk",
			RecordIndexMode:  "full",
		},
		Log: Log{
			Level:  "warn",
			Format: "json",
			Output: "stderr",
			File: LogFile{
				Path:       ".trustdb/logs/trustdb.log",
				MaxSizeMB:  256,
				MaxBackups: 16,
				MaxAgeDays: 30,
				Compress:   true,
			},
			Async: LogAsync{
				BufferSize: 8192,
			},
		},
	}
}

func (c Config) Redacted() Config {
	c.Keys.ClientPrivate = redact(c.Keys.ClientPrivate)
	c.Keys.ClientPublic = redact(c.Keys.ClientPublic)
	c.Keys.ServerPrivate = redact(c.Keys.ServerPrivate)
	c.Keys.ServerPublic = redact(c.Keys.ServerPublic)
	c.Keys.RegistryPrivate = redact(c.Keys.RegistryPrivate)
	c.Keys.RegistryPublic = redact(c.Keys.RegistryPublic)
	return c
}

func redact(value string) string {
	if value == "" {
		return ""
	}
	return "<redacted>"
}

func (c Config) Validate() error {
	if c.Paths.DataDir == "" {
		return fmt.Errorf("paths.data_dir is required")
	}
	if c.Paths.KeyRegistry == "" {
		return fmt.Errorf("paths.key_registry is required")
	}
	if c.Paths.WAL == "" {
		return fmt.Errorf("paths.wal is required")
	}
	if c.Paths.ProofDir == "" {
		return fmt.Errorf("paths.proof_dir is required")
	}
	if c.Identity.Tenant == "" {
		return fmt.Errorf("identity.tenant is required")
	}
	if c.Server.ID == "" {
		return fmt.Errorf("server.id is required")
	}
	if c.Server.KeyID == "" {
		return fmt.Errorf("server.key_id is required")
	}
	if c.Server.Listen == "" {
		return fmt.Errorf("server.listen is required")
	}
	if c.Server.QueueSize <= 0 {
		return fmt.Errorf("server.queue_size must be greater than 0")
	}
	if c.Server.Workers <= 0 {
		return fmt.Errorf("server.workers must be greater than 0")
	}
	if _, err := time.ParseDuration(c.Server.ReadTimeout); err != nil {
		return fmt.Errorf("server.read_timeout must be a valid duration: %w", err)
	}
	if _, err := time.ParseDuration(c.Server.WriteTimeout); err != nil {
		return fmt.Errorf("server.write_timeout must be a valid duration: %w", err)
	}
	if _, err := time.ParseDuration(c.Server.ShutdownTimeout); err != nil {
		return fmt.Errorf("server.shutdown_timeout must be a valid duration: %w", err)
	}
	if c.Registry.KeyID == "" {
		return fmt.Errorf("registry.key_id is required")
	}
	if c.Batch.QueueSize <= 0 {
		return fmt.Errorf("batch.queue_size must be greater than 0")
	}
	if c.Batch.MaxRecords <= 0 {
		return fmt.Errorf("batch.max_records must be greater than 0")
	}
	if _, err := time.ParseDuration(c.Batch.MaxDelay); err != nil {
		return fmt.Errorf("batch.max_delay must be a valid duration: %w", err)
	}
	switch strings.ToLower(c.Batch.ProofMode) {
	case "", "inline", "async", "on_demand":
	default:
		return fmt.Errorf("batch.proof_mode must be one of inline, async, or on_demand")
	}
	switch strings.ToLower(c.Anchor.Scope) {
	case "", "global":
	default:
		return fmt.Errorf("anchor.scope must be global")
	}
	if _, err := time.ParseDuration(c.Anchor.MaxDelay); err != nil {
		return fmt.Errorf("anchor.max_delay must be a valid duration: %w", err)
	}
	if c.History.TileSize == 0 {
		return fmt.Errorf("history.tile_size must be greater than 0")
	}
	if c.History.HotWindowLeaves == 0 {
		return fmt.Errorf("history.hot_window_leaves must be greater than 0")
	}
	switch strings.ToLower(c.Backup.Compression) {
	case "", "gzip", "none":
	default:
		return fmt.Errorf("backup.compression must be gzip or none")
	}
	switch strings.ToLower(c.Proofstore.ArtifactSyncMode) {
	case "", "chunk", "batch":
	default:
		return fmt.Errorf("proofstore.artifact_sync_mode must be chunk or batch")
	}
	switch strings.ToLower(c.Proofstore.RecordIndexMode) {
	case "", "full", "no_storage_tokens", "time_only":
	default:
		return fmt.Errorf("proofstore.record_index_mode must be one of full, no_storage_tokens, or time_only")
	}

	switch strings.ToLower(c.Log.Level) {
	case "", "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("log.level must be one of debug, info, warn, warning, error")
	}
	switch strings.ToLower(c.Log.Format) {
	case "", "json", "console", "text":
	default:
		return fmt.Errorf("log.format must be json, console, or text")
	}
	switch strings.ToLower(c.Log.Output) {
	case "", "stderr", "file", "both":
	default:
		return fmt.Errorf("log.output must be stderr, file, or both")
	}
	if strings.EqualFold(c.Log.Output, "file") || strings.EqualFold(c.Log.Output, "both") {
		if c.Log.File.Path == "" {
			return fmt.Errorf("log.file.path is required when log.output is file or both")
		}
	}
	if c.Log.File.MaxSizeMB <= 0 {
		return fmt.Errorf("log.file.max_size_mb must be greater than 0")
	}
	if c.Log.File.MaxBackups < 0 {
		return fmt.Errorf("log.file.max_backups must be greater than or equal to 0")
	}
	if c.Log.File.MaxAgeDays < 0 {
		return fmt.Errorf("log.file.max_age_days must be greater than or equal to 0")
	}
	if c.Log.Async.BufferSize <= 0 {
		return fmt.Errorf("log.async.buffer_size must be greater than 0")
	}
	return nil
}
