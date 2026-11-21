package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/im/app"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// DefaultMaxUploadBytes is the per-file upload cap (10MB).
const DefaultMaxUploadBytes = 10 << 20

// FileService handles attachment HTTP parsing and delegates storage,
// authorization and lookup to the IM application layer.
type FileService struct {
	v1.UnimplementedFileServiceServer
	uc             *app.FileService
	log            *log.Helper
	MaxUploadBytes int64
}

func NewFileService(uc *app.FileService, logger log.Logger) *FileService {
	if uc.MaxUpload == 0 {
		uc.MaxUpload = DefaultMaxUploadBytes
	}
	return &FileService{uc: uc, log: log.NewHelper(logger), MaxUploadBytes: DefaultMaxUploadBytes}
}

func (s *FileService) UploadHTTP(httpCtx khttp.Context) error {
	h := httpCtx.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		return nil, s.handleUpload(ctx, httpCtx)
	})
	_, err := h(httpCtx, nil)
	return err
}

func (s *FileService) handleUpload(ctx context.Context, httpCtx khttp.Context) error {
	w, r := httpCtx.Response(), httpCtx.Request()
	userID := fileUserIDFromContext(ctx)
	if userID == 0 {
		http.Error(w, `{"message":"authentication required"}`, http.StatusUnauthorized)
		return nil
	}
	var reader io.Reader
	var name, mime string
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(s.MaxUploadBytes); err != nil {
			http.Error(w, `{"message":"invalid multipart form"}`, http.StatusBadRequest)
			return nil
		}
		fh, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"message":"file field is required"}`, http.StatusBadRequest)
			return nil
		}
		defer fh.Close()
		reader, name, mime = fh, header.Filename, header.Header.Get("Content-Type")
	} else {
		reader = r.Body
		name = r.URL.Query().Get("name")
		mime = ct
	}
	if name == "" {
		name = "unnamed"
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	data, err := io.ReadAll(io.LimitReader(reader, s.MaxUploadBytes+1))
	if err != nil {
		http.Error(w, `{"message":"failed to read upload"}`, http.StatusBadRequest)
		return nil
	}
	if int64(len(data)) > s.MaxUploadBytes {
		http.Error(w, fmt.Sprintf(`{"message":"file exceeds %d byte limit"}`, s.MaxUploadBytes), http.StatusRequestEntityTooLarge)
		return nil
	}
	file, err := s.uc.StoreFile(ctx, userID, name, mime, data)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrFileTooLarge):
			http.Error(w, `{"message":"file too large"}`, http.StatusRequestEntityTooLarge)
		default:
			s.log.Errorf("file upload failed: user_id=%d err=%v", userID, err)
			http.Error(w, `{"message":"failed to store file"}`, http.StatusInternalServerError)
		}
		return nil
	}
	reply := &v1.UploadFileReply{FileId: file.ID, Url: "/v1/files/" + file.ID, Name: file.Name, Size: file.Size, Mime: file.Mime}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(reply)
	return nil
}

func (s *FileService) ShareHTTP(httpCtx khttp.Context) error {
	h := httpCtx.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		return nil, s.handleShare(ctx, httpCtx)
	})
	_, err := h(httpCtx, nil)
	return err
}

func (s *FileService) handleShare(ctx context.Context, httpCtx khttp.Context) error {
	w := httpCtx.Response()
	userID := fileUserIDFromContext(ctx)
	if userID <= 0 {
		http.Error(w, `{"message":"authentication required"}`, http.StatusUnauthorized)
		return nil
	}
	var req struct {
		UserIDs []int64 `json:"user_ids"`
		Enabled bool    `json:"enabled"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, httpCtx.Request().Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || len(req.UserIDs) == 0 || len(req.UserIDs) > 100 {
		http.Error(w, `{"message":"invalid share request"}`, http.StatusBadRequest)
		return nil
	}
	fileID := httpCtx.Vars().Get("file_id")
	if err := s.uc.Share(ctx, userID, fileID, req.UserIDs, req.Enabled); err != nil {
		switch {
		case errors.Is(err, app.ErrFileNotFound):
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		case errors.Is(err, app.ErrFileDenied):
			http.Error(w, `{"message":"only the owner can share this file"}`, http.StatusForbidden)
		default:
			http.Error(w, `{"message":"failed to update sharing"}`, http.StatusInternalServerError)
		}
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"file_id": fileID, "updated": len(req.UserIDs), "enabled": req.Enabled})
	return nil
}

func (s *FileService) DownloadHTTP(httpCtx khttp.Context) error {
	h := httpCtx.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		return nil, s.handleDownload(ctx, httpCtx)
	})
	_, err := h(httpCtx, nil)
	return err
}

func (s *FileService) handleDownload(ctx context.Context, httpCtx khttp.Context) error {
	w := httpCtx.Response()
	userID := fileUserIDFromContext(ctx)
	if userID <= 0 {
		http.Error(w, `{"message":"authentication required"}`, http.StatusUnauthorized)
		return nil
	}
	fileID := httpCtx.Vars().Get("file_id")
	file, err := s.uc.GetFile(ctx, userID, fileID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrFileNotFound):
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		case errors.Is(err, app.ErrFileDenied):
			http.Error(w, `{"message":"file access denied"}`, http.StatusForbidden)
		default:
			s.log.Errorf("file download failed: file_id=%s err=%v", fileID, err)
			http.Error(w, `{"message":"failed to load file"}`, http.StatusInternalServerError)
		}
		return nil
	}
	w.Header().Set("Content-Type", file.Mime)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", file.Size))
	disposition := "attachment"
	if strings.HasPrefix(file.Mime, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename*=UTF-8''%s`, disposition, urlQueryEscape(file.Name)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(file.Data); err != nil {
		s.log.Warnf("file download write failed: file_id=%s err=%v", fileID, err)
	}
	return nil
}

func fileUserIDFromContext(ctx context.Context) int64 { return extractUserIDFromContext(ctx) }

func urlQueryEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
