// Package sdk is the public Go client for TrustDB.
//
// It exposes domain-level helpers for building signed file and structured log
// claims, submitting them to a TrustDB server one-by-one, in batches, or from
// bounded streams, fetching proof artifacts, exporting .sproof files, and
// verifying proof material without hand-writing HTTP calls.
package sdk
