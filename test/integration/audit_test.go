package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"nonoka-im/internal/audit"
	platformdata "nonoka-im/internal/data/platform"
)

func TestAudit_AppendOnlyRedactedAndPurgeable(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	store := platformdata.NewStore(ts.data.DB())
	ctx := t.Context()
	if err := store.RecordAuditEvent(ctx, audit.Event{
		ActorUserID: 42, Action: "agent.publish", ResourceType: "agent", ResourceID: "7",
		After:   map[string]any{"name": "fixture", "instructionSha256": "abc", "modelId": 1},
		Outcome: "success", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("record audit: %v", err)
	}
	rows, total, err := store.ListAuditEvents(ctx, audit.Filter{}, 10, 0)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("list audit: total=%d rows=%d err=%v", total, len(rows), err)
	}
	if rows[0].After["instructionSha256"] != "abc" {
		t.Fatalf("audit details not preserved: %+v", rows[0].After)
	}

	// The DB trigger must reject mutation of an audit row.
	err = ts.data.DB().WithContext(ctx).Exec(`UPDATE audit_events SET action = 'tampered' WHERE id = ?`, rows[0].ID).Error
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only update rejection, got %v", err)
	}
	err = ts.data.DB().WithContext(ctx).Exec(`DELETE FROM audit_events WHERE id = ?`, rows[0].ID).Error
	if err == nil || !strings.Contains(err.Error(), "retention purge") {
		t.Fatalf("expected delete guard, got %v", err)
	}
	err = ts.data.DB().WithContext(ctx).Exec(`TRUNCATE audit_events`).Error
	if err == nil || !strings.Contains(err.Error(), "retention purge") {
		t.Fatalf("expected truncate guard, got %v", err)
	}

	// Retention purge is allowed through the dedicated transaction guard.
	n, err := store.PurgeAuditEvents(ctx, time.Now().Add(time.Hour), 100)
	if err != nil || n != 1 {
		t.Fatalf("purge: n=%d err=%v", n, err)
	}
}

func TestAudit_APIRequiresAdminAndRecordsMetadata(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	adminToken, adminID := registerAndLogin(t, "audit-admin", "123456")
	if err := ts.openFGA.SetPlatformAdmin(t.Context(), adminID, true); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	userToken, _ := registerAndLogin(t, "audit-user", "123456")

	// Non-admin cannot read the audit log.
	resp := httpGet(t, testBaseURL+"/v1/admin/audit-events", userToken)
	resp.Body.Close()
	if resp.StatusCode != 403 && resp.StatusCode != 401 {
		t.Fatalf("non-admin audit access = %d", resp.StatusCode)
	}

	// A sensitive action produces an audit row with request metadata.
	create := httpPostAuth(t, testBaseURL+"/v1/agents", adminToken, map[string]any{
		"name": "audited-agent", "instruction": "be audited", "modelId": testModelID,
	})
	create.Body.Close()
	if create.StatusCode != http.StatusOK {
		t.Fatalf("create agent = %d", create.StatusCode)
	}
	resp = httpGet(t, testBaseURL+"/v1/admin/audit-events?action=agent.create", adminToken)
	body := readAllString(t, resp)
	if resp.StatusCode != 200 || !strings.Contains(body, `"agent.create"`) || !strings.Contains(body, `"requestId"`) {
		t.Fatalf("audit API: %d %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "be audited") {
		t.Fatalf("audit leaked instruction text: %s", body)
	}
}

func readAllString(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	buf := new(strings.Builder)
	_, _ = io.Copy(buf, resp.Body)
	return buf.String()
}
