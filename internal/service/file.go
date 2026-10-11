package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/filestore"

	"github.com/go-kratos/kratos/v2/log"
	jwtMiddleware "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// DefaultMaxUploadBytes is the per-file upload cap (10MB).
const DefaultMaxUploadBytes = 10 << 20

// FileService stores and serves message attachments (images, files) in MongoDB.
type FileService struct {
	v1.UnimplementedFileServiceServer
	db             *mongo.Database
	files          *mongo.Collection
	log            *log.Helper
	MaxUploadBytes int64
	authorizer     authz.Authorizer
}

// NewFileService creates a FileService backed by the given MongoDB database.
func NewFileService(db *mongo.Database, logger log.Logger) *FileService {
	return NewFileServiceWithAuthorizer(db, nil, logger)
}

func NewFileServiceWithAuthorizer(db *mongo.Database, authorizer authz.Authorizer, logger log.Logger) *FileService {
	return &FileService{
		db:             db,
		files:          db.Collection(filestore.CollectionFiles),
		log:            log.NewHelper(logger),
		MaxUploadBytes: DefaultMaxUploadBytes,
		authorizer:     authorizer,
	}
}

// UploadHTTP handles POST /v1/files. Accepts either multipart/form-data with
// a "file" field or a raw request body (?name=<filename> for the filename).
// Requires JWT auth: the handler runs inside the server middleware chain so
// the JWT selector applies.
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

	fileID := uuid.NewString()
	doc := filestore.File{
		ID:        fileID,
		Name:      name,
		Mime:      mime,
		Size:      int64(len(data)),
		OwnerID:   userID,
		Data:      data,
		CreatedAt: time.Now(),
	}
	if _, err := s.files.InsertOne(ctx, doc); err != nil {
		s.log.Errorf("file insert failed: file_id=%s, err=%v", fileID, err)
		http.Error(w, `{"message":"failed to store file"}`, http.StatusInternalServerError)
		return nil
	}
	if writer, ok := s.authorizer.(interface {
		SetFileOwner(context.Context, string, int64) error
	}); ok {
		if err := writer.SetFileOwner(ctx, fileID, userID); err != nil {
			s.log.Errorf("file authorization write failed: file_id=%s, err=%v", fileID, err)
			http.Error(w, `{"message":"failed to authorize file"}`, http.StatusInternalServerError)
			return nil
		}
	}

	s.log.Debugf("file uploaded: file_id=%s, name=%s, size=%d, user_id=%d", fileID, name, len(data), userID)
	reply := &v1.UploadFileReply{
		FileId: fileID,
		Url:    "/v1/files/" + fileID,
		Name:   name,
		Size:   int64(len(data)),
		Mime:   mime,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(reply)
	return nil
}

// ShareHTTP handles POST /v1/files/{file_id}/share. The file owner grants or
// revokes explicit viewer access for other users; group/topic visibility is
// managed automatically when the file is shared in conversations.
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
	fileID := httpCtx.Vars().Get("file_id")
	doc, err := filestore.Read(ctx, s.db, fileID)
	if errors.Is(err, filestore.ErrNotFound) {
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		return nil
	}
	if err != nil {
		s.log.Errorf("file lookup failed: file_id=%s, err=%v", fileID, err)
		http.Error(w, `{"message":"failed to load file"}`, http.StatusInternalServerError)
		return nil
	}
	if doc.OwnerID != userID {
		http.Error(w, `{"message":"only the owner can share this file"}`, http.StatusForbidden)
		return nil
	}
	var req struct {
		UserIDs []int64 `json:"user_ids"`
		Enabled bool    `json:"enabled"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, httpCtx.Request().Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || len(req.UserIDs) == 0 {
		http.Error(w, `{"message":"invalid share request"}`, http.StatusBadRequest)
		return nil
	}
	if len(req.UserIDs) > 100 {
		http.Error(w, `{"message":"too many viewers"}`, http.StatusBadRequest)
		return nil
	}
	sharer, ok := s.authorizer.(interface {
		SetFileViewer(context.Context, string, int64, bool) error
	})
	if !ok {
		http.Error(w, `{"message":"file sharing unavailable"}`, http.StatusInternalServerError)
		return nil
	}
	for _, target := range req.UserIDs {
		if target <= 0 || target == userID {
			http.Error(w, `{"message":"invalid target user"}`, http.StatusBadRequest)
			return nil
		}
		// Revocation takes effect in OpenFGA first, before any projection.
		if err := sharer.SetFileViewer(ctx, fileID, target, req.Enabled); err != nil {
			s.log.Errorf("file viewer write failed: file_id=%s, user_id=%d, err=%v", fileID, target, err)
			http.Error(w, `{"message":"failed to update sharing"}`, http.StatusInternalServerError)
			return nil
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"file_id": fileID, "updated": len(req.UserIDs), "enabled": req.Enabled})
	return nil
}

// DownloadHTTP handles GET /v1/files/{file_id}. Downloads require an
// authenticated owner; unguessable IDs are not treated as authorization.
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

	doc, err := filestore.Read(ctx, s.db, fileID)
	if errors.Is(err, filestore.ErrNotFound) {
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
		return nil
	}
	if err != nil {
		s.log.Errorf("file lookup failed: file_id=%s, err=%v", fileID, err)
		http.Error(w, `{"message":"failed to load file"}`, http.StatusInternalServerError)
		return nil
	}
	denied := s.authorizer == nil || s.authorizer.CanReadFile(ctx, userID, fileID) != nil
	if denied {
		http.Error(w, `{"message":"file access denied"}`, http.StatusForbidden)
		return nil
	}

	w.Header().Set("Content-Type", doc.Mime)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", doc.Size))
	disposition := "attachment"
	if strings.HasPrefix(doc.Mime, "image/") {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename*=UTF-8''%s`, disposition, urlQueryEscape(doc.Name)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(doc.Data); err != nil {
		s.log.Warnf("file download write failed: file_id=%s, err=%v", fileID, err)
	}
	return nil
}

func fileUserIDFromContext(ctx context.Context) int64 {
	claims, ok := jwtMiddleware.FromContext(ctx)
	if !ok {
		return 0
	}
	if mapClaims, ok := claims.(jwt5.MapClaims); ok {
		if uid, ok := mapClaims["user_id"].(float64); ok {
			return int64(uid)
		}
	}
	return 0
}

// urlQueryEscape percent-encodes a filename for the Content-Disposition
// filename* parameter (RFC 5987).
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
