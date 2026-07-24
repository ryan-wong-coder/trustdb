// Package sdk is the public Go client for TrustDB.
//
// It exposes domain-level helpers for building signed file and structured log
// claims, submitting them to a TrustDB server one-by-one, in batches, or from
// bounded streams, fetching proof artifacts, exporting .sproof files, and
// verifying proof material without hand-writing HTTP calls.
//
// Every client and identity is bound to exactly one explicit cryptographic
// suite. NewClient and NewINTLV1Identity are the INTL_V1 convenience path.
// CN_SM_V1 callers use NewClientForSuite and NewCNSMV1Identity, or
// NewCallbackSigner when an SM2 private key must remain inside an HSM, SDF,
// KMS, or remote signing service. Requests, responses, status notifications,
// batches, streams, NATS outcomes, and offline evidence fail closed on a suite
// mismatch.
package sdk
