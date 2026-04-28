package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ryan-wong-coder/trustdb/internal/anchor"
	"github.com/ryan-wong-coder/trustdb/internal/proofstore"
	"github.com/ryan-wong-coder/trustdb/internal/trusterr"
	"github.com/spf13/cobra"
)

// newAnchorCommand groups anchor-sink-related maintenance subcommands
// that run outside of `trustdb serve`. These tools operate directly on
// a proofstore and therefore MUST NOT be invoked while the server is
// running against the same store (serve opens the store exclusively).
func newAnchorCommand(rt *runtimeConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "anchor",
		Short: "Maintenance tools for the external anchor layer (L5)",
	}
	cmd.AddCommand(newAnchorExportCommand(rt))
	cmd.AddCommand(newAnchorUpgradeCommand(rt))
	return cmd
}

func newAnchorExportCommand(rt *runtimeConfig) *cobra.Command {
	var metastoreKindStr, metastorePath, proofDir, outPath, format string
	var treeSize uint64
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export an STHAnchorResult for offline L5 verification",
		RunE: func(cmd *cobra.Command, args []string) error {
			if treeSize == 0 {
				return usageError("--tree-size is required")
			}
			store, closeFn, err := openProofStoreForCLI(metastoreKindStr, metastorePath, proofDir, rt.cfg.Paths.ProofDir)
			if err != nil {
				return err
			}
			defer closeFn()

			result, ok, err := store.GetSTHAnchorResult(context.Background(), treeSize)
			if err != nil {
				return err
			}
			if !ok {
				return trusterr.New(trusterr.CodeNotFound,
					fmt.Sprintf("no STHAnchorResult for tree_size=%d", treeSize))
			}
			resolvedFormat, err := writeExportObject(rt, outPath, format, result)
			if err != nil {
				return err
			}
			if outPath == "" {
				return nil
			}
			return rt.writeJSON(struct {
				TreeSize     uint64 `json:"tree_size"`
				SinkName     string `json:"sink_name"`
				AnchorID     string `json:"anchor_id"`
				AnchorResult string `json:"anchor_result"`
				Format       string `json:"format"`
			}{
				TreeSize:     result.TreeSize,
				SinkName:     result.SinkName,
				AnchorID:     result.AnchorID,
				AnchorResult: outPath,
				Format:       resolvedFormat,
			})
		},
	}
	addProofStoreFlags(cmd, &metastoreKindStr, &metastorePath, &proofDir)
	cmd.Flags().Uint64Var(&treeSize, "tree-size", 0, "STH tree size to export (required)")
	cmd.Flags().StringVar(&outPath, "out", "", "write anchor result to file (default format: cbor when --out is set, json otherwise)")
	cmd.Flags().StringVar(&format, "format", "", "output format: json or cbor")
	return cmd
}

// anchorUpgradeReport is the JSON document emitted by
// `trustdb anchor upgrade`. Operators can sanity-check it in CI or
// pipe it into jq to track calendar-by-calendar upgrade progress over
// time. Fields kept intentionally flat so the document is stable and
// easy to diff across runs.
type anchorUpgradeReport struct {
	TreeSize  uint64                    `json:"tree_size"`
	SinkName  string                    `json:"sink_name"`
	AnchorID  string                    `json:"anchor_id"`
	Changed   bool                      `json:"changed"`
	DryRun    bool                      `json:"dry_run"`
	InspectAt int64                     `json:"inspected_at_unix_nano"`
	Calendars []anchor.OtsUpgradeResult `json:"calendars"`
	Persisted bool                      `json:"persisted"`
	PrevProof int                       `json:"prev_proof_bytes"`
	NewProof  int                       `json:"new_proof_bytes"`
}

func newAnchorUpgradeCommand(rt *runtimeConfig) *cobra.Command {
	var metastoreKindStr, metastorePath, proofDir, userAgent, timeoutText string
	var treeSize uint64
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Refresh OpenTimestamps pending timestamps and persist any calendar-side upgrades",
		Long: strings.TrimSpace(`
Re-queries every calendar that previously accepted this STH/global root digest and,
if a calendar has folded the commitment into a Bitcoin block, replaces the
stored raw_timestamp with the upgraded bytes. The STHAnchorResult is
then written back via proofstore.MarkSTHAnchorPublished.

Use --dry-run to preview the calendar responses without persisting anything
back to the proof store. The command is safe to re-run: unchanged calendars
are silently skipped, previously-failed calendars are never re-submitted.
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if treeSize == 0 {
				return usageError("--tree-size is required")
			}
			path := strings.TrimSpace(metastorePath)
			if path == "" {
				path = strings.TrimSpace(proofDir)
			}
			if path == "" {
				return usageError("--metastore-path or --proof-dir is required")
			}
			kind := proofstore.Backend(strings.TrimSpace(metastoreKindStr))
			if kind == "" {
				kind = proofstore.BackendFile
			}
			var timeout time.Duration
			if s := strings.TrimSpace(timeoutText); s != "" {
				d, err := time.ParseDuration(s)
				if err != nil {
					return trusterr.Wrap(trusterr.CodeInvalidArgument, "parse --timeout", err)
				}
				timeout = d
			}

			ctx := context.Background()
			store, err := proofstore.Open(proofstore.Config{Kind: kind, Path: path})
			if err != nil {
				return trusterr.Wrap(trusterr.CodeInternal, "open proofstore", err)
			}
			defer func() { _ = store.Close() }()

			ar, ok, err := store.GetSTHAnchorResult(ctx, treeSize)
			if err != nil {
				return err
			}
			if !ok {
				return trusterr.New(trusterr.CodeNotFound,
					fmt.Sprintf("no STHAnchorResult for tree_size=%d (is --anchor-sink=ots enabled and has this STH been anchored yet?)", treeSize))
			}
			if ar.SinkName != anchor.OtsSinkName {
				return trusterr.New(trusterr.CodeFailedPrecondition,
					fmt.Sprintf("STH tree_size=%d was anchored by sink=%q, not ots; upgrade only supports the ots sink", treeSize, ar.SinkName))
			}

			prevProofLen := len(ar.Proof)
			updated, summary, err := anchor.UpgradeAnchorResult(ctx, ar, anchor.OtsUpgradeOptions{
				Timeout:   timeout,
				UserAgent: userAgent,
			})
			if err != nil {
				return err
			}

			report := anchorUpgradeReport{
				TreeSize:  ar.TreeSize,
				SinkName:  ar.SinkName,
				AnchorID:  ar.AnchorID,
				Changed:   summary.Changed,
				DryRun:    dryRun,
				InspectAt: summary.InspectedAt,
				Calendars: summary.Calendars,
				PrevProof: prevProofLen,
				NewProof:  len(updated.Proof),
			}

			if summary.Changed && !dryRun {
				if err := store.MarkSTHAnchorPublished(ctx, updated); err != nil {
					return trusterr.Wrap(trusterr.CodeDataLoss, "persist upgraded anchor result", err)
				}
				report.Persisted = true
			}
			return rt.writeJSON(report)
		},
	}
	cmd.Flags().Uint64Var(&treeSize, "tree-size", 0, "STH tree size to upgrade (required)")
	cmd.Flags().StringVar(&metastoreKindStr, "metastore", "", "proof store backend: file (default) or pebble")
	cmd.Flags().StringVar(&metastorePath, "metastore-path", "", "proof store path; falls back to --proof-dir when empty")
	cmd.Flags().StringVar(&proofDir, "proof-dir", "", "proof store root directory (file backend). Ignored when --metastore-path is set")
	cmd.Flags().StringVar(&userAgent, "user-agent", "", "override the HTTP User-Agent sent to calendars (empty = default)")
	cmd.Flags().StringVar(&timeoutText, "timeout", "", "per-calendar GET timeout (default 30s)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "query calendars but do not persist upgraded bytes")
	return cmd
}
