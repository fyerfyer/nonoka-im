package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
	SigningKey     []byte
	SignedTTL      time.Duration
}

func NewFileService(uc *app.FileService, logger log.Logger) *FileService {
	if uc.MaxUpload == 0 {
		uc.MaxUpload = DefaultMaxUploadBytes
	}
	helper := log.NewHelper(logger)
	key := strings.TrimSpace(os.Getenv("MEDIA_URL_SECRET"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("AGENT_SECRET_KEY"))
		if key != "" {
			helper.Warn("MEDIA_URL_SECRET is not set; falling back to AGENT_SECRET_KEY (set a dedicated secret in production)")
		}
	}
	var signingKey []byte
	if len(key) >= 16 {
		sum := sha256.Sum256([]byte(key))
		signingKey = sum[:]
	} else {
		helper.Warn("MEDIA_URL_SECRET/AGENT_SECRET_KEY is not configured; signed media URLs are disabled")
	}
	return &FileService{
		uc: uc, log: helper, MaxUploadBytes: DefaultMaxUploadBytes,
		SigningKey: signingKey, SignedTTL: 10 * time.Minute,
	}
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
		// ParseMultipartForm buffers the whole request and spills to temporary
		// files. Bound the body before parsing so a huge upload cannot fill the
		// disk before the size check below.
		r.Body = http.MaxBytesReader(w, r.Body, s.MaxUploadBytes+(1<<20))
		if err := r.ParseMultipartForm(s.MaxUploadBytes); err != nil {
			http.Error(w, `{"message":"invalid multipart form"}`, http.StatusBadRequest)
			return nil
		}
		defer r.MultipartForm.RemoveAll()
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
		return nil, s.handleDownload(ctx, httpCtx, false)
	})
	_, err := h(httpCtx, nil)
	return err
}

// SignHTTP returns a short-TTL URL that <img>/<a> can use without attaching an
// Authorization header. The signature carries the viewer identity, and the
// download handler still re-checks file authorization so revocation is
// immediate.
func (s *FileService) SignHTTP(httpCtx khttp.Context) error {
	h := httpCtx.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		w := httpCtx.Response()
		userID := fileUserIDFromContext(ctx)
		if userID <= 0 {
			http.Error(w, `{"message":"authentication required"}`, http.StatusUnauthorized)
			return nil, nil
		}
		fileID := httpCtx.Vars().Get("file_id")
		if len(s.SigningKey) == 0 {
			http.Error(w, `{"message":"signed media is disabled"}`, http.StatusServiceUnavailable)
			return nil, nil
		}
		if _, err := s.uc.GetFile(ctx, userID, fileID); err != nil {
			s.writeFileError(w, err)
			return nil, nil
		}
		ttl := s.SignedTTL
		if ttl == 0 {
			ttl = 10 * time.Minute
		}
		exp := time.Now().Add(ttl).Unix()
		sig := s.sign(fileID, userID, exp)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"url":       fmt.Sprintf("/v1/media/%s?uid=%d&exp=%d&sig=%s", urlPathEscape(fileID), userID, exp, sig),
			"expiresAt": time.Unix(exp, 0).UTC(),
		})
		return nil, nil
	})
	_, err := h(httpCtx, nil)
	return err
}

// DownloadSignedHTTP serves a signed media URL. The route is excluded from the
// JWT middleware because browsers cannot attach bearer tokens to <img> src.
func (s *FileService) DownloadSignedHTTP(httpCtx khttp.Context) error {
	h := httpCtx.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		w := httpCtx.Response()
		fileID := httpCtx.Vars().Get("file_id")
		q := httpCtx.Request().URL.Query()
		uid, err := strconv.ParseInt(q.Get("uid"), 10, 64)
		exp, expErr := strconv.ParseInt(q.Get("exp"), 10, 64)
		if len(s.SigningKey) == 0 || err != nil || uid <= 0 || expErr != nil || exp <= 0 || !hmac.Equal([]byte(q.Get("sig")), []byte(s.sign(fileID, uid, exp))) {
			http.Error(w, `{"message":"invalid media signature"}`, http.StatusUnauthorized)
			return nil, nil
		}
		if time.Now().Unix() > exp {
			http.Error(w, `{"message":"media URL expired"}`, http.StatusUnauthorized)
			return nil, nil
		}
		s.writeFile(w, ctx, uid, fileID)
		return nil, nil
	})
	_, err := h(httpCtx, nil)
	return err
}

func (s *FileService) handleDownload(ctx context.Context, httpCtx khttp.Context, signed bool) error {
	w := httpCtx.Response()
	userID := fileUserIDFromContext(ctx)
	if userID <= 0 {
		http.Error(w, `{"message":"authentication required"}`, http.StatusUnauthorized)
		return nil
	}
	_ = signed
	s.writeFile(w, ctx, userID, httpCtx.Vars().Get("file_id"))
	return nil
}

func (s *FileService) writeFile(w http.ResponseWriter, ctx context.Context, userID int64, fileID string) {
	file, err := s.uc.GetFile(ctx, userID, fileID)
	if err != nil {
		s.writeFileError(w, err)
		return
	}
	w.Header().Set("Content-Type", file.Mime)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", file.Size))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	disposition := "attachment"
	// SVG is an image MIME type but can execute script when opened inline.
	baseMime := strings.ToLower(strings.TrimSpace(strings.SplitN(file.Mime, ";", 2)[0]))
	if strings.HasPrefix(baseMime, "image/") && baseMime != "image/svg+xml" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename*=UTF-8''%s`, disposition, urlQueryEscape(file.Name)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(file.Data); err != nil {
		s.log.Warnf("file download write failed: file_id=%s err=%v", fileID, err)
	}
}

func (s *FileService) writeFileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrFileNotFound):
		http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
	case errors.Is(err, app.ErrFileDenied):
		http.Error(w, `{"message":"file access denied"}`, http.StatusForbidden)
	default:
		s.log.Errorf("file download failed: %v", err)
		http.Error(w, `{"message":"failed to load file"}`, http.StatusInternalServerError)
	}
}

func (s *FileService) sign(fileID string, userID, exp int64) string {
	mac := hmac.New(sha256.New, s.SigningKey)
	fmt.Fprintf(mac, "%s\n%d\n%d", fileID, userID, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

func urlPathEscape(s string) string {
	return urlQueryEscape(s)
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
