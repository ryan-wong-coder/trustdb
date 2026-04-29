package anchor

import (
	"context"

	"github.com/ryan-wong-coder/trustdb/internal/model"
	"github.com/ryan-wong-coder/trustdb/internal/proofstore"
)

// API exposes the two anchor reads the HTTP layer needs. It is a thin
// wrapper around a proofstore.Store so tests and CLI code can use an
// in-memory store without pulling in the full Service. The wrapper
// also makes the intent explicit in serve_cmd wiring: the worker
// writes the outbox, the API only reads it.
type API struct {
	Store proofstore.Store
}

// NewAPI constructs an API backed by the provided store.
func NewAPI(store proofstore.Store) *API { return &API{Store: store} }

func (a *API) AnchorResult(ctx context.Context, treeSize uint64) (model.STHAnchorResult, bool, error) {
	return a.Store.GetSTHAnchorResult(ctx, treeSize)
}

func (a *API) AnchorStatus(ctx context.Context, treeSize uint64) (model.STHAnchorOutboxItem, bool, error) {
	return a.Store.GetSTHAnchorOutboxItem(ctx, treeSize)
}

func (a *API) Anchors(ctx context.Context, opts model.AnchorListOptions) ([]model.STHAnchorOutboxItem, error) {
	return a.Store.ListSTHAnchorsPage(ctx, opts)
}
