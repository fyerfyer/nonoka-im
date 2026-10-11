package skills

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"nonoka-im/internal/agent/runtime"
	"nonoka-im/internal/data"
)

type fakeStore struct {
	rows map[string]*data.Skill
}

func (f *fakeStore) GetSkill(_ context.Context, name string, version int64) (*data.Skill, error) {
	row, ok := f.rows[name]
	if !ok || row.Version != version {
		return nil, gorm.ErrRecordNotFound
	}
	return row, nil
}

func TestBackendListsAndReadsOnlyBoundSkills(t *testing.T) {
	store := &fakeStore{rows: map[string]*data.Skill{
		"summarizer": {Name: "summarizer", Version: 2, Content: "---\nname: summarizer\ndescription: Summarize long threads\n---\nUse bullet points.", Published: true},
		"ghost":      {Name: "ghost", Version: 1, Content: "---\nname: ghost\n---\nHidden.", Published: false},
	}}
	backend := NewBackend(store, []runtime.SkillBinding{{Name: "summarizer", Version: 2}, {Name: "ghost", Version: 1}, {Name: "absent", Version: 1}})

	listed, err := backend.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != "summarizer" || listed[0].Description != "Summarize long threads" {
		t.Fatalf("unexpected list: %+v", listed)
	}

	skill, err := backend.Get(context.Background(), "summarizer")
	if err != nil {
		t.Fatal(err)
	}
	if skill.Content != "Use bullet points." {
		t.Fatalf("body not stripped: %q", skill.Content)
	}
	if skill.Context != "" || skill.Agent != "" || skill.Model != "" {
		t.Fatalf("unsafe front matter not cleared: %+v", skill.FrontMatter)
	}

	if _, err := backend.Get(context.Background(), "ghost"); err == nil {
		t.Fatalf("unpublished skill readable")
	}
	if _, err := backend.Get(context.Background(), "absent"); err == nil {
		t.Fatalf("unbound skill readable")
	}
}

func TestParseFrontMatterWithoutMarker(t *testing.T) {
	fm, body := ParseFrontMatter("plain instructions")
	if fm.Name != "" || body != "plain instructions" {
		t.Fatalf("unexpected parse: %+v %q", fm, body)
	}
}
