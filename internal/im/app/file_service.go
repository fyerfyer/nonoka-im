package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/authz"
	"nonoka-im/internal/im/domain"

	"github.com/google/uuid"
)

// FileService stores attachments and enforces file authorization.
type FileService struct {
	Store      FileStore
	Authorizer authz.FileAuthorizer
	MaxUpload  int64
}

var (
	ErrFileNotFound = errors.New("file not found")
	ErrFileTooLarge = errors.New("file too large")
	ErrFileDenied   = errors.New("file access denied")
)

func (s *FileService) StoreFile(ctx context.Context, userID int64, name, mime string, data []byte) (FileObject, error) {
	if userID <= 0 {
		return FileObject{}, ErrAuthRequired
	}
	if s.MaxUpload > 0 && int64(len(data)) > s.MaxUpload {
		return FileObject{}, ErrFileTooLarge
	}
	if name == "" {
		name = "unnamed"
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	file := FileObject{
		ID: uuid.NewString(), Name: name, Mime: mime, Size: int64(len(data)),
		OwnerID: userID, Data: data, CreatedAt: time.Now(),
	}
	if err := s.Store.Put(ctx, file); err != nil {
		return FileObject{}, fmt.Errorf("store file: %w", err)
	}
	if s.Authorizer != nil {
		if err := s.Authorizer.SetFileOwner(ctx, file.ID, userID); err != nil {
			return FileObject{}, fmt.Errorf("authorize file: %w", err)
		}
	}
	return file, nil
}

func (s *FileService) GetFile(ctx context.Context, userID int64, fileID string) (FileObject, error) {
	if userID <= 0 {
		return FileObject{}, ErrAuthRequired
	}
	file, err := s.Store.Get(ctx, fileID)
	if err != nil {
		return FileObject{}, ErrFileNotFound
	}
	if s.Authorizer == nil || s.Authorizer.CanReadFile(ctx, userID, fileID) != nil {
		return FileObject{}, ErrFileDenied
	}
	return file, nil
}

// ReadFile is the agent-facing read path. Authorization is checked by the
// caller (tool layer) because agent tools authorize the resource separately.
func (s *FileService) ReadFile(ctx context.Context, fileID string) (FileObject, error) {
	file, err := s.Store.Get(ctx, fileID)
	if err != nil {
		return FileObject{}, ErrFileNotFound
	}
	return file, nil
}

func (s *FileService) Share(ctx context.Context, userID int64, fileID string, targetIDs []int64, enabled bool) error {
	if userID <= 0 {
		return ErrAuthRequired
	}
	file, err := s.Store.Get(ctx, fileID)
	if err != nil {
		return ErrFileNotFound
	}
	if file.OwnerID != userID {
		return ErrFileDenied
	}
	if s.Authorizer == nil {
		return ErrFileDenied
	}
	for _, target := range targetIDs {
		if target <= 0 || target == userID {
			return fmt.Errorf("invalid target user")
		}
		if err := s.Authorizer.SetFileViewer(ctx, fileID, target, enabled); err != nil {
			return err
		}
	}
	return nil
}

// ShareToTopic grants topic participants read access to attachments referenced
// by an image/file message. It is best-effort and never blocks sending.
func (s *FileService) ShareToTopic(ctx context.Context, userID int64, msgType domain.MessageType, content []byte, topic string) {
	if msgType != domain.MessageTypeImage && msgType != domain.MessageTypeFile {
		return
	}
	if s.Authorizer == nil {
		return
	}
	var meta struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(content, &meta); err != nil || meta.URL == "" {
		return
	}
	fileID := strings.TrimPrefix(meta.URL, "/v1/files/")
	if fileID == meta.URL || strings.ContainsAny(fileID, "/?#") {
		return
	}
	_ = s.Authorizer.SetFileSharedTopic(ctx, fileID, topic, true)
}
