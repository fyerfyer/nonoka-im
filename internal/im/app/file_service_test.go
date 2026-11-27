package app

import (
	"context"
	"encoding/json"
	"testing"

	"nonoka-im/internal/im/domain"
)

type fileStoreStub struct{ files map[string]FileObject }

func (s *fileStoreStub) Put(_ context.Context, file FileObject) error {
	s.files[file.ID] = file
	return nil
}
func (s *fileStoreStub) Get(_ context.Context, fileID string) (FileObject, error) {
	return s.GetMeta(context.Background(), fileID)
}
func (s *fileStoreStub) GetMeta(_ context.Context, fileID string) (FileObject, error) {
	file, ok := s.files[fileID]
	if !ok {
		return FileObject{}, ErrFileNotFound
	}
	file.Data = nil
	return file, nil
}

type fileAuthorizerStub struct{ shared map[string]string }

func (a *fileAuthorizerStub) CanReadFile(context.Context, int64, string) error  { return nil }
func (a *fileAuthorizerStub) SetFileOwner(context.Context, string, int64) error { return nil }
func (a *fileAuthorizerStub) SetFileViewer(context.Context, string, int64, bool) error {
	return nil
}
func (a *fileAuthorizerStub) SetFileSharedTopic(_ context.Context, fileID, topic string, enabled bool) error {
	if a.shared == nil {
		a.shared = map[string]string{}
	}
	if enabled {
		a.shared[fileID] = topic
	} else {
		delete(a.shared, fileID)
	}
	return nil
}

func TestFileService_ShareToTopicRequiresOwnership(t *testing.T) {
	store := &fileStoreStub{files: map[string]FileObject{
		"owner-file": {ID: "owner-file", OwnerID: 7},
	}}
	authz := &fileAuthorizerStub{}
	svc := &FileService{Store: store, Authorizer: authz}

	content, _ := json.Marshal(map[string]string{"file_id": "owner-file", "url": "/v1/files/owner-file"})
	svc.ShareToTopic(context.Background(), 8, domain.MessageTypeImage, content, "grp_team")
	if _, ok := authz.shared["owner-file"]; ok {
		t.Fatal("non-owner was able to share a file into a topic")
	}

	svc.ShareToTopic(context.Background(), 7, domain.MessageTypeImage, content, "grp_team")
	if got := authz.shared["owner-file"]; got != "grp_team" {
		t.Fatalf("owner share topic = %q, want grp_team", got)
	}
}
