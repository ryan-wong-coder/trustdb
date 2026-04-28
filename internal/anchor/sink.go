// Package anchor implements the L5 (Anchored) proof layer. It defines a
// generic Sink interface for external notaries (file, Certificate
// Transparency, public blockchains, ...) and a background worker that
// drains the proofstore anchor outbox and records the publish result.
//
// The separation is intentional: commit-time code only writes a single
// AnchorOutboxItem and is done. All retry, backoff, observability and
// sink-specific quirks live behind the Sink contract, so swapping a
// FileSink for a CtLogSink is a configuration change rather than a
// code change in the batch pipeline.
package anchor

import (
	"context"
	"errors"

	"github.com/ryan-wong-coder/trustdb/internal/model"
)

// Sink is the one-method contract every external notary must satisfy.
// Implementations are expected to be safe for concurrent use because
// the worker uses a small pool of goroutines; a Sink that cannot
// handle concurrency must serialise internally.
//
// Name returns a short stable identifier ("file", "ct", "bitcoin",
// ...) recorded in every STHAnchorOutboxItem / STHAnchorResult so that a
// proof verifier can pick the right Sink-specific proof parser.
//
// Publish must return either:
//
//   - (result, nil)    on success; the worker stores the result and
//     transitions the outbox item to AnchorStatePublished.
//   - (_, ErrPermanent) wrapped error on a non-retryable failure
//     (e.g. schema rejected, signature algorithm unknown). The worker
//     marks the item Failed and never retries.
//   - (_, any other error) on a transient failure. The worker bumps
//     the attempts counter and schedules the item for a later retry.
//
// Implementations should use errors.Is(err, anchor.ErrPermanent) when
// they need to declare a permanent failure so the worker can
// distinguish the two classes without string matching.
type Sink interface {
	Name() string
	Publish(ctx context.Context, sth model.SignedTreeHead) (model.STHAnchorResult, error)
}

// ErrPermanent is a sentinel wrapped by Sink implementations to signal
// that retrying will not help. The worker only checks it via
// errors.Is, so callers can wrap it freely with fmt.Errorf("%w: ...",
// anchor.ErrPermanent) and preserve their own message.
var ErrPermanent = errors.New("anchor: permanent sink failure")
