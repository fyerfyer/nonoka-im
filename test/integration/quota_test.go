package integration

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"nonoka-im/internal/agent/domain"
	agentdata "nonoka-im/internal/data/agent"
)

func TestQuota_EnqueueAndClaimRespectDailyAndConcurrency(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	store := agentdata.NewStore(ts.data.DB())
	ctx := t.Context()
	day := time.Now().UTC().Truncate(24 * time.Hour)
	userID := int64(990101)
	agentID := int64(990102)
	limits := domain.QuotaLimits{MaxConcurrentRuns: 1, DailyRuns: 1, DailyTokens: 100000, DailyCostUSD: 10, MaxToolTurns: 8}

	makeRun := func(seq uint64) *domain.AgentRun {
		return &domain.AgentRun{
			EventID: fmt.Sprintf("quota-evt-%d-%d", time.Now().UnixNano(), seq), AgentID: agentID,
			ActorUserID: userID, Topic: fmt.Sprintf("p2p_%d_%d", userID, agentID),
			MsgID: int64(seq), TopicSeq: seq, Status: domain.RunQueued, CreatedAt: time.Now(),
		}
	}

	first := makeRun(1)
	created, quotaErr, err := store.EnqueueRunWithQuota(ctx, first, day, time.Now().Add(time.Hour), limits, limits, 3, 200)
	if err != nil || !created || quotaErr != nil {
		t.Fatalf("first enqueue: created=%v quota=%v err=%v", created, quotaErr, err)
	}
	second := makeRun(2)
	created, quotaErr, err = store.EnqueueRunWithQuota(ctx, second, day, time.Now().Add(time.Hour), limits, limits, 3, 200)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if created || quotaErr == nil || quotaErr.Metric != "runs" {
		t.Fatalf("daily run limit not enforced: created=%v quota=%+v", created, quotaErr)
	}

	// A second run outside the daily-run limit still cannot start while the
	// first holds the only agent concurrency slot.
	other := makeRun(3)
	other.EventID = fmt.Sprintf("quota-other-%d", time.Now().UnixNano())
	if _, err := store.EnqueueRun(ctx, other); err != nil {
		t.Fatalf("raw enqueue: %v", err)
	}
	claimed, ok, err := store.ClaimRunWithQuota(ctx, first.ID, time.Minute, day, time.Now().Add(24*time.Hour), limits, limits)
	if err != nil || !ok || claimed == nil {
		t.Fatalf("claim first: run=%+v ok=%v err=%v", claimed, ok, err)
	}
	_, ok, err = store.ClaimRunWithQuota(ctx, other.ID, time.Minute, day, time.Now().Add(24*time.Hour), limits, limits)
	if err != nil || ok {
		t.Fatalf("concurrency limit bypassed: ok=%v err=%v", ok, err)
	}
	if err := store.FailQueuedRun(ctx, other.ID, "quota_exceeded", "quota", "concurrency test cleanup", nil); err != nil {
		t.Fatalf("fail queued run: %v", err)
	}
}

func TestQuota_QueueFullDefersAndCountsOnce(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	store := agentdata.NewStore(ts.data.DB())
	ctx := t.Context()
	day := time.Now().UTC().Truncate(24 * time.Hour)
	userID := int64(990301)
	agentID := int64(990302)
	limits := domain.QuotaLimits{DailyRuns: 5}
	topic := fmt.Sprintf("p2p_%d_%d", userID, agentID)
	makeRun := func(seq uint64) *domain.AgentRun {
		return &domain.AgentRun{
			EventID: fmt.Sprintf("defer-evt-%d-%d", time.Now().UnixNano(), seq), AgentID: agentID,
			ActorUserID: userID, Topic: topic,
			MsgID: int64(seq), TopicSeq: seq, Status: domain.RunQueued, Kind: domain.RunKindIM, CreatedAt: time.Now(),
		}
	}

	first := makeRun(1)
	created, quotaErr, err := store.EnqueueRunWithQuota(ctx, first, day, time.Now().Add(time.Hour), limits, limits, 3, 1)
	if err != nil || !created || quotaErr != nil {
		t.Fatalf("first enqueue: created=%v quota=%v err=%v", created, quotaErr, err)
	}
	second := makeRun(2)
	created, quotaErr, err = store.EnqueueRunWithQuota(ctx, second, day, time.Now().Add(time.Hour), limits, limits, 3, 1)
	if err != nil || created || quotaErr == nil || quotaErr.Metric != "queued_runs" {
		t.Fatalf("expected global queue denial: created=%v quota=%+v err=%v", created, quotaErr, err)
	}

	retryAt := time.Now().Add(time.Second)
	if err := store.EnqueueDeferredRun(ctx, second, day, retryAt, 1); err != nil {
		t.Fatalf("defer run: %v", err)
	}
	deferred, err := store.GetRun(ctx, second.ID)
	if err != nil || deferred == nil || deferred.Status != domain.RunQueued || deferred.NextAttemptAt == nil {
		t.Fatalf("deferred run not queued for replay: run=%+v err=%v", deferred, err)
	}
	usage, err := store.GetUsage(ctx, day, domain.QuotaScopeUser, fmt.Sprintf("%d", userID))
	if err != nil || usage.Runs != 2 {
		t.Fatalf("deferred run was not counted once: usage=%+v err=%v", usage, err)
	}

	// A Kafka redelivery of the same event must not create or count it twice.
	if err := store.EnqueueDeferredRun(ctx, second, day, retryAt, 1); err != nil {
		t.Fatalf("duplicate defer: %v", err)
	}
	usage, err = store.GetUsage(ctx, day, domain.QuotaScopeUser, fmt.Sprintf("%d", userID))
	if err != nil || usage.Runs != 2 {
		t.Fatalf("duplicate defer double-counted: usage=%+v err=%v", usage, err)
	}
}

func TestQuota_AdminAPIAndAudit(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	adminToken, adminID := registerAndLogin(t, "quota-admin", "123456")
	if err := ts.openFGA.SetPlatformAdmin(t.Context(), adminID, true); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	userToken, _ := registerAndLogin(t, "quota-user", "123456")

	put := httpPutAuth(t, testBaseURL+"/v1/admin/quotas/user/4242", adminToken, map[string]any{
		"maxConcurrentRuns": 1, "dailyRuns": 7, "dailyTokens": 1000, "dailyCostUsd": 1.5, "maxToolTurns": 4,
	})
	put.Body.Close()
	if put.StatusCode != 200 {
		t.Fatalf("admin update quota: %d", put.StatusCode)
	}
	list := httpGet(t, testBaseURL+"/v1/admin/quotas?scope=user", adminToken)
	body := readAllString(t, list)
	if list.StatusCode != 200 || !strings.Contains(body, `"4242"`) || !strings.Contains(body, `"dailyRuns":7`) {
		t.Fatalf("list quotas: %d %s", list.StatusCode, body)
	}

	deny := httpPutAuth(t, testBaseURL+"/v1/admin/quotas/user/4242", userToken, map[string]any{"dailyRuns": 9})
	deny.Body.Close()
	if deny.StatusCode != 403 {
		t.Fatalf("non-admin quota update: %d", deny.StatusCode)
	}

	audit := httpGet(t, testBaseURL+"/v1/admin/audit-events?action=quota.update", adminToken)
	auditBody := readAllString(t, audit)
	if audit.StatusCode != 200 || !strings.Contains(auditBody, "quota.update") {
		t.Fatalf("quota audit: %d %s", audit.StatusCode, auditBody)
	}

	reset := httpDeleteAuth(t, testBaseURL+"/v1/admin/quotas/user/4242", adminToken)
	reset.Body.Close()
	if reset.StatusCode != 200 {
		t.Fatalf("reset quota: %d", reset.StatusCode)
	}
}

func TestQuota_BudgetBlocksBeforeModel(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	store := agentdata.NewStore(ts.data.DB())
	ctx := t.Context()
	day := time.Now().UTC().Truncate(24 * time.Hour)
	userID := int64(990201)
	agentID := int64(990202)
	run := &domain.AgentRun{
		EventID: fmt.Sprintf("budget-evt-%d", time.Now().UnixNano()), AgentID: agentID,
		ActorUserID: userID, Topic: fmt.Sprintf("p2p_%d_%d", userID, agentID),
		MsgID: 1, TopicSeq: 1, Status: domain.RunQueued, CreatedAt: time.Now(),
	}
	created, quotaErr, err := store.EnqueueRunWithQuota(ctx, run, day, time.Now().Add(time.Hour), domain.QuotaLimits{DailyRuns: 10}, domain.QuotaLimits{DailyRuns: 10}, 3, 200)
	if err != nil || !created || quotaErr != nil {
		t.Fatalf("enqueue budget run: created=%v quota=%v err=%v", created, quotaErr, err)
	}
	// Record spend over the user budget directly in the daily aggregate.
	if _, err := store.RecordUsageEvent(ctx, day, domain.UsageEvent{
		RunID: run.ID, AttemptID: "seed", Step: 1, Kind: domain.RunKindIM,
		AgentID: agentID, ActorUserID: userID, Topic: run.Topic,
		ModelID: 1, ModelKey: "fixture/model", Provider: "fixture",
		InputTokens: 100, OutputTokens: 100, CostUSD: 50,
	}); err != nil {
		t.Fatalf("seed usage: %v", err)
	}

	_, ok, err := store.ClaimRunWithQuota(ctx, run.ID, time.Minute, day, time.Now().Add(24*time.Hour),
		domain.QuotaLimits{MaxConcurrentRuns: 2, DailyRuns: 10, DailyCostUSD: 1},
		domain.QuotaLimits{MaxConcurrentRuns: 2, DailyRuns: 10, DailyCostUSD: 100})
	if ok || err == nil {
		t.Fatalf("expected budget denial, ok=%v err=%v", ok, err)
	}
	var budgetErr *domain.BudgetError
	if !errors.As(err, &budgetErr) || budgetErr.Metric != "cost" {
		t.Fatalf("expected cost budget error, got %T %v", err, err)
	}
}
