package skills

import (
	"context"
	"testing"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

type fakeStore struct {
	rows map[uint64]*domain.SkillVersion
}

func (f *fakeStore) GetSkillVersion(_ context.Context, skillID uint64, version int64) (*domain.SkillVersion, error) {
	row, ok := f.rows[skillID]
	if !ok || row.Version != version {
		return nil, domain.ErrSkillVersionNotFound
	}
	return row, nil
}

func TestBackendListsAndReadsOnlyBoundSkills(t *testing.T) {
	store := &fakeStore{rows: map[uint64]*domain.SkillVersion{
		1: {SkillID: 1, Version: 2, Content: "---\nname: summarizer\ndescription: Summarize long threads\n---\nUse bullet points.", Status: domain.SkillVersionPublished},
		2: {SkillID: 2, Version: 1, Content: "---\nname: ghost\ndescription: Hidden\n---\nHidden.", Status: domain.SkillVersionDeprecated},
	}}
	backend := NewBackend(store, []port.SkillBinding{{SkillID: 1, Version: 2, Name: "summarizer"}, {SkillID: 2, Version: 1, Name: "ghost"}})

	listed, err := backend.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Name != "summarizer" || listed[0].Description != "Summarize long threads" {
		t.Fatalf("unexpected list: %+v", listed)
	}
	skill, err := backend.Get(context.Background(), "summarizer")
	if err != nil {
		t.Fatal(err)
	}
	if skill.Content != "Use bullet points." {
		t.Fatalf("body not stripped: %q", skill.Content)
	}
	if _, err := backend.Get(context.Background(), "absent"); err == nil {
		t.Fatalf("unbound skill readable")
	}
}

func TestBackendRejectsChecksumMismatch(t *testing.T) {
	store := &fakeStore{rows: map[uint64]*domain.SkillVersion{
		1: {SkillID: 1, Version: 1, Content: "---\nname: s\ndescription: d\n---\nbody", Checksum: "new", Status: domain.SkillVersionPublished},
	}}
	backend := NewBackend(store, []port.SkillBinding{{SkillID: 1, Version: 1, Checksum: "old"}})
	if _, err := backend.List(context.Background()); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}

func TestParseFrontMatterHandlesYAMLValues(t *testing.T) {
	content := "---\nname: quoted\ndescription: \"Use a: b, then c\"\n---\nBody line."
	fm, body := ParseFrontMatter(content)
	if fm.Name != "quoted" || fm.Description != "Use a: b, then c" {
		t.Fatalf("yaml values not parsed: %+v", fm)
	}
	if body != "Body line." {
		t.Fatalf("body not stripped: %q", body)
	}
}

func TestParseFrontMatterWithoutMarker(t *testing.T) {
	fm, body := ParseFrontMatter("plain instructions")
	if fm.Name != "" || body != "plain instructions" {
		t.Fatalf("unexpected parse: %+v %q", fm, body)
	}
}
