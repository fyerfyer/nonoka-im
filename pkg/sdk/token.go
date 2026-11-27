package sdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TokenSourceFunc adapts a function to TokenSource.
type TokenSourceFunc func(ctx context.Context) (string, error)

func (f TokenSourceFunc) Token(ctx context.Context) (string, error) { return f(ctx) }

// StaticToken is a fixed bearer token.
type StaticToken string

func (t StaticToken) Token(context.Context) (string, error) { return string(t), nil }

// PasswordTokenSource logs in with username/password, caches the token pair
// and refreshes it automatically before expiry.
type PasswordTokenSource struct {
	BaseURL  string
	Username string
	Password string
	DeviceID string
	HTTP     *http.Client

	mu        sync.Mutex
	access    string
	refresh   string
	userID    int64
	expiresAt time.Time
}

func NewPasswordTokenSource(baseURL, username, password, deviceID string) *PasswordTokenSource {
	if deviceID == "" {
		deviceID = "sdk-default"
	}
	return &PasswordTokenSource{
		BaseURL: strings.TrimRight(baseURL, "/"), Username: username, Password: password,
		DeviceID: deviceID, HTTP: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *PasswordTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.access != "" && (s.expiresAt.IsZero() || time.Until(s.expiresAt) > 30*time.Second) {
		return s.access, nil
	}
	if s.refresh != "" && s.userID > 0 {
		if err := s.refreshLocked(ctx); err == nil {
			return s.access, nil
		}
	}
	if err := s.loginLocked(ctx); err != nil {
		return "", err
	}
	return s.access, nil
}

func (s *PasswordTokenSource) Refresh(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refresh == "" || s.userID <= 0 {
		if err := s.loginLocked(ctx); err != nil {
			return "", err
		}
		return s.access, nil
	}
	if err := s.refreshLocked(ctx); err != nil {
		if loginErr := s.loginLocked(ctx); loginErr != nil {
			return "", err
		}
	}
	return s.access, nil
}

func (s *PasswordTokenSource) loginLocked(ctx context.Context) error {
	var reply struct {
		UserID       jsonInt64 `json:"userId"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refreshToken"`
	}
	body := map[string]any{"username": s.Username, "password": s.Password, "deviceId": s.DeviceID}
	if err := postPublicJSON(ctx, s.HTTP, s.BaseURL+"/v1/auth/login", body, &reply); err != nil {
		return err
	}
	s.access, s.refresh, s.userID = reply.Token, reply.RefreshToken, int64(reply.UserID)
	s.expiresAt = tokenExpiry(reply.Token)
	return nil
}

func (s *PasswordTokenSource) refreshLocked(ctx context.Context) error {
	var reply struct {
		UserID       jsonInt64 `json:"userId"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refreshToken"`
	}
	body := map[string]any{"userId": s.userID, "deviceId": s.DeviceID, "refreshToken": s.refresh}
	if err := postPublicJSON(ctx, s.HTTP, s.BaseURL+"/v1/auth/refresh", body, &reply); err != nil {
		return err
	}
	s.access, s.refresh = reply.Token, reply.RefreshToken
	if reply.UserID > 0 {
		s.userID = int64(reply.UserID)
	}
	s.expiresAt = tokenExpiry(reply.Token)
	return nil
}

func postPublicJSON(ctx context.Context, client *http.Client, url string, body, out any) error {
	if client == nil {
		client = http.DefaultClient
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return decodeAPIError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func tokenExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Expires int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Expires <= 0 {
		return time.Time{}
	}
	return time.Unix(claims.Expires, 0)
}

func userIDFromToken(token string) int64 {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0
	}
	var claims struct {
		UserID jsonInt64 `json:"user_id"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0
	}
	return int64(claims.UserID)
}

func (s *PasswordTokenSource) String() string {
	return fmt.Sprintf("PasswordTokenSource(%s)", s.Username)
}
