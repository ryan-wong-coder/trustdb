package sdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/sproof"
)

const defaultHTTPTimeout = 15 * time.Second

type Transport interface {
	Endpoint() string
	CheckHealth(context.Context) HealthStatus
	SubmitSignedClaim(context.Context, SignedClaim) (SubmitResult, error)
	GetRecord(context.Context, string) (RecordIndex, error)
	ListRecords(context.Context, ListRecordsOptions) (RecordPage, error)
	GetProofBundle(context.Context, string) (ProofBundle, error)
	ListRoots(context.Context, int) ([]BatchRoot, error)
	LatestRoot(context.Context) (BatchRoot, error)
	GetGlobalProof(context.Context, string) (GlobalLogProof, error)
	GetAnchor(context.Context, uint64) (AnchorStatus, error)
	LatestSTH(context.Context) (SignedTreeHead, error)
	GetSTH(context.Context, uint64) (SignedTreeHead, error)
	MetricsRaw(context.Context) (string, error)
}

type Client struct {
	transport Transport
}

type Option func(*httpTransport)

func WithHTTPClient(client *http.Client) Option {
	return func(t *httpTransport) {
		if client != nil {
			t.httpClient = client
		}
	}
}

func WithUserAgent(userAgent string) Option {
	return func(t *httpTransport) {
		t.userAgent = strings.TrimSpace(userAgent)
	}
}

func NewClient(baseURL string, opts ...Option) (*Client, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return nil, errors.New("sdk: server url is empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("sdk: parse server url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("sdk: server url must include scheme and host: %s", trimmed)
	}
	transport := &httpTransport{
		baseURL: strings.TrimRight(trimmed, "/"),
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		userAgent: "trustdb-go-sdk",
	}
	for _, apply := range opts {
		apply(transport)
	}
	return NewClientWithTransport(transport)
}

func NewClientWithTransport(transport Transport) (*Client, error) {
	if transport == nil {
		return nil, errors.New("sdk: transport is nil")
	}
	return &Client{transport: transport}, nil
}

func (c *Client) BaseURL() string {
	return c.transport.Endpoint()
}

func (c *Client) Close() error {
	closer, ok := c.transport.(interface{ Close() error })
	if !ok {
		return nil
	}
	return closer.Close()
}

func (c *Client) Health(ctx context.Context) error {
	status := c.CheckHealth(ctx)
	if status.OK {
		return nil
	}
	return &Error{Op: "health", URL: c.BaseURL(), StatusCode: status.StatusCode, Message: status.Error}
}

func (c *Client) CheckHealth(ctx context.Context) HealthStatus {
	return c.transport.CheckHealth(ctx)
}

func (c *Client) SubmitFile(ctx context.Context, raw io.Reader, id Identity, opts FileClaimOptions) (SubmitResult, error) {
	signed, err := BuildSignedFileClaim(raw, id, opts)
	if err != nil {
		return SubmitResult{}, err
	}
	result, err := c.SubmitSignedClaim(ctx, signed)
	if err != nil {
		return SubmitResult{}, err
	}
	result.SignedClaim = signed
	return result, nil
}

func (c *Client) SubmitSignedClaim(ctx context.Context, signed SignedClaim) (SubmitResult, error) {
	return c.transport.SubmitSignedClaim(ctx, signed)
}

func (c *Client) GetRecord(ctx context.Context, recordID string) (RecordIndex, error) {
	return c.transport.GetRecord(ctx, recordID)
}

func (c *Client) ListRecords(ctx context.Context, opts ListRecordsOptions) (RecordPage, error) {
	return c.transport.ListRecords(ctx, opts)
}

func (c *Client) ListRoots(ctx context.Context, limit int) ([]BatchRoot, error) {
	return c.transport.ListRoots(ctx, limit)
}

func (c *Client) LatestRoot(ctx context.Context) (BatchRoot, error) {
	return c.transport.LatestRoot(ctx)
}

func (c *Client) GetProofBundle(ctx context.Context, recordID string) (ProofBundle, error) {
	return c.transport.GetProofBundle(ctx, recordID)
}

func (c *Client) GetGlobalProof(ctx context.Context, batchID string) (GlobalLogProof, error) {
	return c.transport.GetGlobalProof(ctx, batchID)
}

func (c *Client) GetAnchor(ctx context.Context, treeSize uint64) (AnchorStatus, error) {
	return c.transport.GetAnchor(ctx, treeSize)
}

func (c *Client) LatestSTH(ctx context.Context) (SignedTreeHead, error) {
	return c.transport.LatestSTH(ctx)
}

func (c *Client) GetSTH(ctx context.Context, treeSize uint64) (SignedTreeHead, error) {
	return c.transport.GetSTH(ctx, treeSize)
}

func (c *Client) ExportSingleProof(ctx context.Context, recordID string) (SingleProof, error) {
	bundle, err := c.GetProofBundle(ctx, recordID)
	if err != nil {
		return SingleProof{}, err
	}
	opts := sproof.Options{ExportedAtUnixN: time.Now().UTC().UnixNano()}
	global, err := c.GetGlobalProof(ctx, bundle.CommittedReceipt.BatchID)
	if err != nil {
		if !IsUnavailable(err) {
			return SingleProof{}, fmt.Errorf("sdk: fetch global proof: %w", err)
		}
		return sproof.New(bundle, opts)
	}
	opts.GlobalProof = &global
	anchor, err := c.GetAnchor(ctx, global.STH.TreeSize)
	if err != nil {
		if !IsUnavailable(err) {
			return SingleProof{}, fmt.Errorf("sdk: fetch anchor: %w", err)
		}
		return sproof.New(bundle, opts)
	}
	opts.AnchorResult = anchor.Result
	return sproof.New(bundle, opts)
}

func (c *Client) WriteSingleProofFile(ctx context.Context, recordID, path string) error {
	proof, err := c.ExportSingleProof(ctx, recordID)
	if err != nil {
		return err
	}
	return sproof.WriteFile(path, proof)
}

func (c *Client) MetricsRaw(ctx context.Context) (string, error) {
	return c.transport.MetricsRaw(ctx)
}
