// Package store adapts the PostgreSQL agent store to the Eino CheckPointStore
// interface so interrupted runs survive worker restarts.
package store

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// CheckpointStore is the persistence surface for Eino execution state.
type CheckpointStore interface {
	GetCheckpoint(ctx context.Context, checkpointID string) ([]byte, bool, error)
	SaveCheckpoint(ctx context.Context, runID uint64, checkpointID string, state []byte, ttl time.Duration) error
}

// CheckPoints implements adk.CheckPointStore (Get/Set of opaque state bytes)
// on top of the agent repository. The Eino checkpoint ID carries the run ID
// ("agent-run-<id>"), which is parsed back on Set because the interface only
// passes the checkpoint ID.
type CheckPoints struct {
	Store CheckpointStore
	TTL   time.Duration // checkpoint retention; zero defaults to 24h
}

func (c *CheckPoints) ttl() time.Duration {
	if c.TTL <= 0 {
		return 24 * time.Hour
	}
	return c.TTL
}

func (c *CheckPoints) Get(ctx context.Context, checkPointID string) ([]byte, bool, error) {
	return c.Store.GetCheckpoint(ctx, checkPointID)
}

func (c *CheckPoints) Set(ctx context.Context, checkPointID string, checkPoint []byte) error {
	return c.Store.SaveCheckpoint(ctx, runIDFromCheckpointID(checkPointID), checkPointID, checkPoint, c.ttl())
}

func runIDFromCheckpointID(checkPointID string) uint64 {
	id, err := strconv.ParseUint(strings.TrimPrefix(checkPointID, "agent-run-"), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
