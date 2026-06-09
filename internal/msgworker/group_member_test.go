package msgworker

import (
	"context"
	"testing"
)

func TestInMemoryGroupMemberService(t *testing.T) {
	svc := NewInMemoryGroupMemberService()
	ctx := context.Background()

	// Add members
	svc.AddGroupMember(ctx, "group1", 100)
	svc.AddGroupMember(ctx, "group1", 200)
	svc.AddGroupMember(ctx, "group1", 300)
	svc.AddGroupMember(ctx, "group2", 100)

	// Get members of group1
	members, err := svc.GetGroupMembers(ctx, "group1")
	if err != nil {
		t.Fatalf("get group members failed: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}

	// Check all members exist
	memberSet := make(map[int64]bool)
	for _, m := range members {
		memberSet[m] = true
	}
	if !memberSet[100] || !memberSet[200] || !memberSet[300] {
		t.Fatalf("expected members 100, 200, 300, got %v", members)
	}

	// Get members of non-existent group
	members, err = svc.GetGroupMembers(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("get nonexistent group members failed: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected 0 members for nonexistent group, got %d", len(members))
	}

	// Remove member
	svc.RemoveGroupMember(ctx, "group1", 200)
	members, err = svc.GetGroupMembers(ctx, "group1")
	if err != nil {
		t.Fatalf("get group members after remove failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members after remove, got %d", len(members))
	}

	t.Log("InMemoryGroupMemberService test passed")
}

func TestExtractGroupID(t *testing.T) {
	tests := []struct {
		topic    string
		expected string
		wantErr  bool
	}{
		{"grp_123", "123", false},
		{"grp_abc", "abc", false},
		{"grp_42_test", "42_test", false},
		{"p2p_1_2", "", true},
		{"invalid", "", true},
		{"grp_", "", false},
	}

	for _, tt := range tests {
		groupID, err := ExtractGroupID(tt.topic)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("expected error for topic %s", tt.topic)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for topic %s: %v", tt.topic, err)
		}
		if groupID != tt.expected {
			t.Fatalf("expected groupID=%s, got %s for topic %s", tt.expected, groupID, tt.topic)
		}
	}

	t.Log("ExtractGroupID test passed")
}
