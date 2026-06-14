package msgworker

import (
	"testing"
)

// TestNewSnowflake_NormalizesNegativeNodeID verifies that a negative node ID is
// normalized into the [0, 1023] range instead of being rejected or aliased to
// another node. This is a regression test for the bug where Go's % operator
// returned a negative value and snowflake.NewNode failed.
func TestNewSnowflake_NormalizesNegativeNodeID(t *testing.T) {
	gen := NewSnowflake(-1)
	if gen == nil || gen.node == nil {
		t.Fatal("expected non-nil IDGenerator")
	}

	id1 := gen.NextID()
	id2 := gen.NextID()
	if id1 == 0 || id2 == 0 {
		t.Fatal("snowflake IDs should be non-zero")
	}
	if id1 == id2 {
		t.Fatal("consecutive snowflake IDs should be unique")
	}
}

// TestNewSnowflake_LargeNodeID verifies that node IDs larger than 1023 are
// wrapped modulo 1024 and still produce valid IDs.
func TestNewSnowflake_LargeNodeID(t *testing.T) {
	gen := NewSnowflake(2048 + 42)
	if gen == nil || gen.node == nil {
		t.Fatal("expected non-nil IDGenerator")
	}
	if gen.NextID() == 0 {
		t.Fatal("expected non-zero snowflake ID")
	}
}
