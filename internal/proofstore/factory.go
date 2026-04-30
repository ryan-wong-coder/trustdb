package proofstore

import (
	"strings"

	pebblestore "github.com/ryan-wong-coder/trustdb/internal/proofstore/pebble"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
)

// Backend enumerates the supported proof store implementations.
type Backend string

const (
	BackendFile   Backend = "file"
	BackendPebble Backend = "pebble"
)

// Config picks the backend and its on-disk location. Path is treated as
// a directory path for both file and Pebble modes; the file backend
// puts manifests/bundles/roots/checkpoint under it, while the Pebble
// backend uses it as the database directory.
type Config struct {
	Kind                         Backend
	Path                         string
	IndexStorageTokens           bool
	IndexStorageTokensConfigured bool
}

// Open constructs a Store using cfg. An empty Kind defaults to the file
// backend so existing deployments that only pass a proof directory keep
// working without any CLI changes.
func Open(cfg Config) (Store, error) {
	if cfg.Path == "" {
		return nil, trusterr.New(trusterr.CodeInvalidArgument, "proofstore path is required")
	}
	switch Backend(strings.ToLower(string(cfg.Kind))) {
	case "", BackendFile:
		return &LocalStore{Root: cfg.Path}, nil
	case BackendPebble:
		if cfg.IndexStorageTokensConfigured {
			return pebblestore.OpenWithOptions(cfg.Path, pebblestore.Options{IndexStorageTokens: cfg.IndexStorageTokens})
		}
		return pebblestore.Open(cfg.Path)
	default:
		return nil, trusterr.New(trusterr.CodeInvalidArgument, "unknown proofstore backend: "+string(cfg.Kind))
	}
}
