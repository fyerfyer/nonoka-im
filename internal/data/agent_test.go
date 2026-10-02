package data

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// setupTestAgentStore connects to a real PostgreSQL for SQL behavior tests.
// Set NONOKA_AGENT_TEST_PG_DSN to override; the test is skipped when no
// PostgreSQL is reachable (the integration suite covers the live runs).
func setupTestAgentStore(t *testing.T) *AgentStore {
	t.Helper()
	dsns := []string{os.Getenv("NONOKA_AGENT_TEST_PG_DSN")}
	if dsns[0] == "" {
		dsns = dsns[:0]
	}
	dsns = append(dsns,
		"host=127.0.0.1 user=postgres password=root dbname=nonoka_im port=5432 sslmode=disable",
		"host=127.0.0.1 user=nonoka password=nonoka dbname=nonoka port=5432 sslmode=disable",
	)
	var db *gorm.DB
	var err error
	for _, dsn := range dsns {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			continue
		}
		sqlDB, serr := db.DB()
		if serr != nil {
			continue
		}
		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		perr := sqlDB.PingContext(pingCtx)
		cancel()
		if perr == nil {
			break
		}
		_ = sqlDB.Close()
		db = nil
	}
	if db == nil {
		t.Skip("PostgreSQL not reachable; set NONOKA_AGENT_TEST_PG_DSN")
	}
	t.Cleanup(func() {
		if sqlDB, cerr := db.DB(); cerr == nil {
			_ = sqlDB.Close()
		}
	})
	return NewAgentStore(&Data{db: db})
}

func TestBlockingRunIgnoresFailedRuns(t *testing.T) {
	store := setupTestAgentStore(t)
	ctx := context.Background()
	agentID, actorID := int64(987000001), int64(987000002)
	topic := fmt.Sprintf("p2p_%d_%d", actorID, agentID)
	base := time.Now().UnixNano()

	session, err := store.GetSession(ctx, agentID, actorID, topic)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	// Clear leftovers from interrupted runs so the session starts empty.
	if err := store.db.Where("session_id = ?", session.ID).Delete(&AgentRun{}).Error; err != nil {
		t.Fatalf("clear session runs: %v", err)
	}
	t.Cleanup(func() {
		_ = store.db.Where("session_id = ?", session.ID).Delete(&AgentRun{}).Error
		_ = store.db.Where("id = ?", session.ID).Delete(&AgentSession{}).Error
	})

	insertRun := func(eventID string, topicSeq uint64, status string) *AgentRun {
		run := &AgentRun{EventID: eventID, AgentID: agentID, SessionID: session.ID, ActorUserID: actorID, Topic: topic, MsgID: int64(base), TopicSeq: topicSeq, Status: status, Attempts: 1, CreatedAt: time.Now()}
		if err := store.db.Create(run).Error; err != nil {
			t.Fatalf("insert run %s: %v", eventID, err)
		}
		return run
	}

	failed := insertRun(fmt.Sprintf("evt-failed-%d", base), 5, "failed")
	later := insertRun(fmt.Sprintf("evt-later-%d", base), 9, "running")

	// A failed run must not block a newer run in the same session.
	blocker, err := store.BlockingRun(ctx, session.ID, 10, later.ID)
	if err != nil {
		t.Fatalf("BlockingRun: %v", err)
	}
	if blocker != nil && blocker.ID == failed.ID {
		t.Fatalf("failed run %d must not block run %d", failed.ID, later.ID)
	}

	// QueuedRuns must surface a queued run even when an earlier run in the
	// same session failed.
	queued := insertRun(fmt.Sprintf("evt-queued-%d", base), 7, "queued")
	defer func() { _ = store.db.Where("id = ?", queued.ID).Delete(&AgentRun{}).Error }()
	runs, err := store.QueuedRuns(ctx, 32)
	if err != nil {
		t.Fatalf("QueuedRuns: %v", err)
	}
	found := false
	for _, r := range runs {
		if r.ID == queued.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("queued run %d not selected by QueuedRuns; failed run must not block replay", queued.ID)
	}

	// A running run with a smaller topic_seq still blocks.
	early := insertRun(fmt.Sprintf("evt-early-%d", base), 3, "running")
	defer func() { _ = store.db.Where("id = ?", early.ID).Delete(&AgentRun{}).Error }()
	blocker, err = store.BlockingRun(ctx, session.ID, 10, later.ID)
	if err != nil {
		t.Fatalf("BlockingRun with running predecessor: %v", err)
	}
	if blocker == nil || blocker.ID != early.ID {
		t.Fatalf("expected running run %d to block, got %+v", early.ID, blocker)
	}
}
