// Package modelsuite validates the cryptographic-suite binding of V2 model
// objects before they cross a wire, WAL, proofstore, or composition boundary.
package modelsuite

import (
	"fmt"

	"github.com/wowtrust/trustdb/internal/cryptosuite"
	"github.com/wowtrust/trustdb/internal/model"
)

func Require(expected cryptosuite.ID, value any) error {
	if _, err := cryptosuite.RequireAvailable(expected); err != nil {
		return err
	}
	require := func(name string, actual ...cryptosuite.ID) error {
		if err := cryptosuite.RequireSame(expected, actual...); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
	switch v := value.(type) {
	case model.ClientClaim:
		return require("client claim", v.CryptoSuite)
	case model.SignedClaim:
		return require("signed claim", v.CryptoSuite, v.Claim.CryptoSuite)
	case model.ServerRecord:
		return require("server record", v.CryptoSuite)
	case model.AcceptedReceipt:
		return require("accepted receipt", v.CryptoSuite)
	case model.CommittedReceipt:
		return require("committed receipt", v.CryptoSuite)
	case model.ProofBundle:
		return require("proof bundle", v.CryptoSuite, v.SignedClaim.CryptoSuite, v.SignedClaim.Claim.CryptoSuite, v.ServerRecord.CryptoSuite, v.AcceptedReceipt.CryptoSuite, v.CommittedReceipt.CryptoSuite)
	case model.RecordIndex:
		return require("record index", v.CryptoSuite)
	case model.RecordStatus:
		return require("record status", v.CryptoSuite)
	case model.StatusRefresh:
		return require("status refresh", v.CryptoSuite)
	case model.BatchRoot:
		return require("batch root", v.CryptoSuite)
	case model.WALCheckpoint:
		return require("WAL checkpoint", v.CryptoSuite)
	case model.BatchManifest:
		return require("batch manifest", v.CryptoSuite)
	case model.BatchTreeLeaf:
		return require("batch tree leaf", v.CryptoSuite)
	case model.BatchTreeNode:
		return require("batch tree node", v.CryptoSuite)
	case model.GlobalLogLeaf:
		return require("global log leaf", v.CryptoSuite)
	case model.GlobalLogNode:
		return require("global log node", v.CryptoSuite)
	case model.GlobalLogState:
		return require("global log state", v.CryptoSuite)
	case model.SignedTreeHead:
		return require("signed tree head", v.CryptoSuite)
	case model.GlobalLogProof:
		return require("global log proof", v.CryptoSuite, v.STH.CryptoSuite)
	case model.GlobalLogTile:
		return require("global log tile", v.CryptoSuite)
	case model.GlobalLogOutboxItem:
		actual := []cryptosuite.ID{v.CryptoSuite, v.BatchRoot.CryptoSuite}
		if v.STH.TreeSize != 0 {
			actual = append(actual, v.STH.CryptoSuite)
		}
		return require("global log outbox", actual...)
	case model.GlobalLogAppend:
		actual := []cryptosuite.ID{v.Leaf.CryptoSuite, v.State.CryptoSuite, v.STH.CryptoSuite}
		for i := range v.Nodes {
			actual = append(actual, v.Nodes[i].CryptoSuite)
		}
		return require("global log append", actual...)
	case model.STHAnchorCandidate:
		return require("STH anchor candidate", v.STH.CryptoSuite)
	case model.STHAnchorResult:
		return require("STH anchor result", v.CryptoSuite, v.STH.CryptoSuite)
	case model.STHAnchorLatestReference:
		if v.SchemaVersion == model.SchemaSTHAnchorLatestEmpty {
			return nil
		}
		return require("STH anchor latest reference", v.CryptoSuite)
	case model.STHAnchorSchedule:
		actual := []cryptosuite.ID{v.CryptoSuite}
		if v.Pending != nil {
			actual = append(actual, v.Pending.Target.CryptoSuite)
		}
		if v.InFlight != nil {
			actual = append(actual, v.InFlight.Target.CryptoSuite)
		}
		return require("STH anchor schedule", actual...)
	case model.L5CoverageCheckpoint:
		return require("L5 coverage checkpoint", v.CryptoSuite)
	case model.IdempotencyDecision:
		return require("idempotency decision", v.CryptoSuite, v.Record.CryptoSuite, v.Accepted.CryptoSuite)
	default:
		return fmt.Errorf("unsupported suite-bound model type %T", value)
	}
}
