package integration

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"nonoka-im/pkg/sdk"
)

func TestSDKResourcesEndToEnd(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	ownerToken, ownerID := registerAndLogin(t, "sdk-resources-owner", "123456")
	otherToken, otherID := registerAndLogin(t, "sdk-resources-other", "123456")
	_ = otherToken

	client := newTestSDKClient(t, ownerToken, "sdk-resources")
	defer client.Close()
	ctx := context.Background()

	// Users.
	users, err := client.Users.Search(ctx, "sdk-resources-other", 10)
	if err != nil || len(users) == 0 || users[0].UserID != otherID {
		t.Fatalf("user search: %+v err=%v", users, err)
	}

	// Groups.
	group, err := client.Groups.Create(ctx, "sdk-resources-group", []int64{otherID})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	groups, err := client.Groups.ListMine(ctx)
	if err != nil || len(groups) == 0 {
		t.Fatalf("list groups: %+v err=%v", groups, err)
	}
	if _, err := client.Groups.Get(ctx, group.GroupID); err != nil {
		t.Fatalf("get group: %v", err)
	}
	members, err := client.Groups.ListMembers(ctx, group.GroupID)
	if err != nil || len(members) != 2 {
		t.Fatalf("group members: %+v err=%v", members, err)
	}
	if err := client.Groups.RemoveMember(ctx, group.GroupID, otherID); err != nil {
		t.Fatalf("remove group member: %v", err)
	}
	if err := client.Groups.AddMember(ctx, group.GroupID, otherID); err != nil {
		t.Fatalf("add group member: %v", err)
	}
	if _, err := client.Users.Presence(ctx, ownerID); err != nil {
		t.Fatalf("presence path: %v", err)
	}
	if err := client.Conversations.MarkRead(ctx, group.Topic); err != nil {
		t.Fatalf("mark read path: %v", err)
	}

	// Files.
	uploaded, err := client.Files.Upload(ctx, "sdk.txt", []byte("hello sdk file"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	data, _, err := client.Files.Download(ctx, uploaded.FileID)
	if err != nil || !bytes.Equal(data, []byte("hello sdk file")) {
		t.Fatalf("download: %q err=%v", data, err)
	}
	if err := client.Files.Share(ctx, uploaded.FileID, []int64{otherID}, true); err != nil {
		t.Fatalf("file share: %v", err)
	}

	// Model catalog and agent tool catalog.
	models, canManage, err := client.Models.List(ctx)
	if err != nil || len(models) == 0 || canManage {
		t.Fatalf("models: %+v canManage=%v err=%v", models, canManage, err)
	}
	presets, tools, err := client.Agents.Tools(ctx)
	if err != nil || len(presets) == 0 || len(tools) == 0 {
		t.Fatalf("agent tools: presets=%d tools=%d err=%v", len(presets), len(tools), err)
	}

	// Agent CRUD, versions and restore.
	agent, err := client.Agents.Create(ctx, sdk.AgentInput{
		Name: "sdk-resources-agent", Instruction: "first sdk instruction",
		ModelID:           testModelID,
		CapabilityPresets: []string{"im.read"}, ToolPolicies: map[string]string{},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	version, err := client.Agents.Publish(ctx, agent.ID, agent.Draft.Revision, "sdk first release")
	if err != nil || version.PublishNote != "sdk first release" {
		t.Fatalf("publish: %+v err=%v", version, err)
	}
	detail, err := client.Agents.Get(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	updated, err := client.Agents.UpdateDraft(ctx, agent.ID, sdk.AgentInput{
		Name: "sdk-resources-agent", Instruction: "second sdk instruction",
		ModelID:           testModelID,
		CapabilityPresets: []string{"im.read"}, ToolPolicies: map[string]string{},
		Revision: detail.Draft.Revision,
	})
	if err != nil || updated.Draft.Instruction != "second sdk instruction" {
		t.Fatalf("update draft: %+v err=%v", updated, err)
	}
	versions, err := client.Agents.Versions(ctx, agent.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v err=%v", versions, err)
	}
	fullVersion, err := client.Agents.Version(ctx, agent.ID, 1)
	if err != nil || fullVersion.Instruction != "first sdk instruction" {
		t.Fatalf("version detail: %+v err=%v", fullVersion, err)
	}
	restored, err := client.Agents.Restore(ctx, agent.ID, 1, updated.Draft.Revision)
	if err != nil || restored.Draft.Instruction != "first sdk instruction" {
		t.Fatalf("restore: %+v err=%v", restored, err)
	}

	// MCP and Skill binding.
	fixture := startRegistryFixture(t)
	mcpServer, err := client.MCP.Register(ctx, sdk.MCPServerInput{
		Name: "sdk-fixture", DisplayName: "SDK Fixture", URL: fixture.URL,
		Transport: "streamable_http", AuthType: "none", Visibility: "private",
	})
	if err != nil {
		t.Fatalf("register mcp: %v", err)
	}
	mcpTools, err := client.MCP.Discover(ctx, mcpServer.ID)
	if err != nil || len(mcpTools) == 0 {
		t.Fatalf("discover mcp: %+v err=%v", mcpTools, err)
	}
	skill, err := client.Skills.Create(ctx, sdk.SkillInput{
		Slug: "sdk-resources-skill", DisplayName: "SDK Skill", Visibility: "private",
		Content: "---\nname: sdk-resources-skill\ndescription: sdk skill\n---\n\nBe concise.\n",
	})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	skillDetail, err := client.Skills.Get(ctx, skill.ID)
	if err != nil || skillDetail.LatestVersion != 1 {
		t.Fatalf("get skill: %+v err=%v", skillDetail, err)
	}

	// Share with a viewer and verify the share list.
	if err := client.Agents.Share(ctx, agent.ID, "user", fmt.Sprint(otherID), "viewer", true); err != nil {
		t.Fatalf("share agent: %v", err)
	}
	shares, err := client.Agents.Shares(ctx, agent.ID)
	if err != nil || len(shares) != 1 || shares[0].Role != "viewer" {
		t.Fatalf("agent shares: %+v err=%v", shares, err)
	}

	// Draft test and run history.
	testRun, err := client.Agents.Test(ctx, agent.ID, "hello sdk test")
	if err != nil || testRun.Kind != "test" {
		t.Fatalf("draft test: %+v err=%v", testRun, err)
	}
	runs, err := client.Agents.Runs(ctx, agent.ID, sdk.ListOptions{})
	if err != nil || len(runs.Items) == 0 {
		t.Fatalf("agent runs: %+v err=%v", runs, err)
	}
	runDetail, err := client.Runs.Get(ctx, testRun.ID)
	if err != nil || runDetail.InputText != "hello sdk test" {
		t.Fatalf("run detail: %+v err=%v", runDetail, err)
	}
	if err := client.Runs.Cancel(ctx, testRun.ID); err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	cancelled, err := client.Runs.Get(ctx, testRun.ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("cancelled run: %+v err=%v", cancelled, err)
	}

	// Approval inbox and one-shot decision via SDK.
	run, approval := newWaitingApprovalRun(t, newAgentStore(ts), agent.ID, ownerID, "p2p_1_2", "sdk-approval-event")
	approvals, err := client.Runs.Approvals(ctx, "pending", sdk.ListOptions{})
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	found := false
	for _, item := range approvals.Items {
		if item.ID == approval.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("approval %d missing from inbox: %+v", approval.ID, approvals.Items)
	}
	approvalDetail, err := client.Runs.Approval(ctx, approval.ID)
	if err != nil || approvalDetail.Tool != "send_message" {
		t.Fatalf("approval detail: %+v err=%v", approvalDetail, err)
	}
	if err := client.Runs.Approve(ctx, approval.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	approved, err := client.Runs.Get(ctx, run.ID)
	if err != nil || approved.Status != "queued" {
		t.Fatalf("approved run should be requeued: %+v err=%v", approved, err)
	}

	// Message pull path is exercised by the realtime test; keep a smoke call.
	if _, err := client.Messages.Pull(ctx, sdk.PullRequest{Topic: group.Topic, Limit: 10}); err != nil {
		t.Fatalf("pull messages: %v", err)
	}
}
