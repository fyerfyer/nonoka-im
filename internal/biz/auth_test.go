package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"nonoka-im/internal/conf"

	"github.com/go-kratos/kratos/v2/errors"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
)

// fakeAuthRepo is an in-memory AuthRepo for unit testing the AuthUsecase.
type fakeAuthRepo struct {
	mu          sync.Mutex
	users       map[string]*User
	usersByID   map[int64]*User
	refresh     map[string]string // "userID:deviceID" -> token hash
	savedTTL    map[string]time.Duration
	nextID      int64
	getUserErr  error
	saveHashErr error
}

func newFakeAuthRepo() *fakeAuthRepo {
	return &fakeAuthRepo{
		users:     make(map[string]*User),
		usersByID: make(map[int64]*User),
		refresh:   make(map[string]string),
		savedTTL:  make(map[string]time.Duration),
		nextID:    1,
	}
}

func (r *fakeAuthRepo) CreateUser(_ context.Context, u *User) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[u.Username]; ok {
		return nil, errors.BadRequest("USERNAME_EXISTS", "username already exists")
	}
	user := &User{ID: r.nextID, Username: u.Username, Password: u.Password}
	r.nextID++
	r.users[user.Username] = user
	r.usersByID[user.ID] = user
	return user, nil
}

func (r *fakeAuthRepo) GetUserByUsername(_ context.Context, username string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.users[username]; ok {
		return u, nil
	}
	return nil, errors.NotFound("USER_NOT_FOUND", "user not found")
}

func (r *fakeAuthRepo) GetUserByID(_ context.Context, id int64) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getUserErr != nil {
		return nil, r.getUserErr
	}
	if u, ok := r.usersByID[id]; ok {
		return u, nil
	}
	return nil, errors.NotFound("USER_NOT_FOUND", "user not found")
}

func (r *fakeAuthRepo) SearchUsersByPrefix(_ context.Context, prefix string, limit int32) ([]*User, error) {
	return nil, nil
}

func (r *fakeAuthRepo) SaveRefreshTokenHash(_ context.Context, userID int64, deviceID, hash string, ttl time.Duration) error {
	if r.saveHashErr != nil {
		return r.saveHashErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refresh[fmt.Sprintf("%d:%s", userID, deviceID)] = hash
	r.savedTTL[fmt.Sprintf("%d:%s", userID, deviceID)] = ttl
	return nil
}

func (r *fakeAuthRepo) GetRefreshTokenHash(_ context.Context, userID int64, deviceID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	hash, ok := r.refresh[fmt.Sprintf("%d:%s", userID, deviceID)]
	if !ok {
		return "", errors.NotFound("REFRESH_TOKEN_NOT_FOUND", "refresh token not found or expired")
	}
	return hash, nil
}

func (r *fakeAuthRepo) DeleteRefreshToken(_ context.Context, userID int64, deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.refresh, fmt.Sprintf("%d:%s", userID, deviceID))
	return nil
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func newTestAuthUsecase(repo AuthRepo) *AuthUsecase {
	return NewAuthUsecase(repo, &conf.Auth{
		JwtSecret:       "unit-test-secret",
		TokenTtl:        durationpb.New(time.Hour),
		RefreshTokenTtl: durationpb.New(30 * 24 * time.Hour),
	})
}

func mustRegister(t *testing.T, uc *AuthUsecase, username, password string) *User {
	t.Helper()
	user, err := uc.Register(context.Background(), username, password)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	return user
}

func TestAuthLogin_IssuesRefreshToken(t *testing.T) {
	repo := newFakeAuthRepo()
	uc := newTestAuthUsecase(repo)
	mustRegister(t, uc, "alice", "123456")

	userID, accessToken, refreshToken, err := uc.Login(context.Background(), "alice", "123456", "dev-1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if userID == 0 || accessToken == "" {
		t.Fatalf("expected non-zero userID and access token, got %d / %q", userID, accessToken)
	}
	if len(refreshToken) != 64 {
		t.Fatalf("expected 64-hex-char refresh token, got %q", refreshToken)
	}

	stored, err := repo.GetRefreshTokenHash(context.Background(), userID, "dev-1")
	if err != nil {
		t.Fatalf("refresh token hash not stored: %v", err)
	}
	if stored != hashToken(refreshToken) {
		t.Fatalf("stored hash does not match sha256(refresh token)")
	}
	if ttl := repo.savedTTL[fmt.Sprintf("%d:%s", userID, "dev-1")]; ttl != 30*24*time.Hour {
		t.Fatalf("expected 30d refresh TTL, got %v", ttl)
	}

	// Logging in again replaces the previous refresh token.
	_, _, refreshToken2, err := uc.Login(context.Background(), "alice", "123456", "dev-1")
	if err != nil {
		t.Fatalf("second Login failed: %v", err)
	}
	if refreshToken2 == refreshToken {
		t.Fatalf("expected a new refresh token on re-login")
	}
	if stored := repo.refresh[fmt.Sprintf("%d:%s", userID, "dev-1")]; stored != hashToken(refreshToken2) {
		t.Fatalf("stored hash was not rotated on re-login")
	}
}

func TestAuthLogin_WrongPassword_NoRefreshToken(t *testing.T) {
	uc := newTestAuthUsecase(newFakeAuthRepo())
	mustRegister(t, uc, "bob", "123456")

	_, _, _, err := uc.Login(context.Background(), "bob", "wrong", "dev-1")
	if !errors.IsUnauthorized(err) {
		t.Fatalf("expected Unauthorized, got %v", err)
	}
}

func TestAuthRefresh_RotatesTokenPair(t *testing.T) {
	repo := newFakeAuthRepo()
	uc := newTestAuthUsecase(repo)
	mustRegister(t, uc, "carol", "123456")

	userID, _, refresh1, err := uc.Login(context.Background(), "carol", "123456", "dev-1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// Valid refresh issues a brand-new pair.
	id2, access2, refresh2, err := uc.Refresh(context.Background(), userID, "dev-1", refresh1)
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}
	if id2 != userID {
		t.Fatalf("expected userID %d, got %d", userID, id2)
	}
	// Note: the access token may be byte-identical to the old one when the
	// refresh happens within the same second (same iat/exp claims), which is
	// fine. The refresh token must always be a fresh random value.
	if refresh2 == refresh1 {
		t.Fatalf("expected a rotated refresh token")
	}
	if access2 == "" || refresh2 == "" {
		t.Fatalf("expected non-empty token pair")
	}

	// Old refresh token is dead after rotation (replay rejected).
	if _, _, _, err := uc.Refresh(context.Background(), userID, "dev-1", refresh1); !errors.IsUnauthorized(err) {
		t.Fatalf("expected Unauthorized for replayed refresh token, got %v", err)
	}

	// New refresh token works.
	if _, _, _, err := uc.Refresh(context.Background(), userID, "dev-1", refresh2); err != nil {
		t.Fatalf("Refresh with rotated token failed: %v", err)
	}
}

func TestAuthRefresh_Rejections(t *testing.T) {
	repo := newFakeAuthRepo()
	uc := newTestAuthUsecase(repo)
	user := mustRegister(t, uc, "dave", "123456")

	if _, _, _, err := uc.Refresh(context.Background(), user.ID, "dev-1", ""); !errors.IsUnauthorized(err) {
		t.Fatalf("expected Unauthorized for empty token, got %v", err)
	}
	// Unknown device: nothing stored.
	if _, _, _, err := uc.Refresh(context.Background(), user.ID, "dev-1", "deadbeef"); !errors.IsUnauthorized(err) {
		t.Fatalf("expected Unauthorized for unknown token, got %v", err)
	}
	// Token from another device must not validate.
	_, _, refreshOther, err := uc.Login(context.Background(), "dave", "123456", "dev-2")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if _, _, _, err := uc.Refresh(context.Background(), user.ID, "dev-1", refreshOther); !errors.IsUnauthorized(err) {
		t.Fatalf("expected Unauthorized for cross-device token, got %v", err)
	}
}

func TestAuthLogout_RevokesRefreshToken(t *testing.T) {
	repo := newFakeAuthRepo()
	uc := newTestAuthUsecase(repo)
	mustRegister(t, uc, "erin", "123456")

	userID, _, refreshToken, err := uc.Login(context.Background(), "erin", "123456", "dev-1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if err := uc.Logout(context.Background(), userID, "dev-1"); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
	if _, _, _, err := uc.Refresh(context.Background(), userID, "dev-1", refreshToken); !errors.IsUnauthorized(err) {
		t.Fatalf("expected Unauthorized after logout, got %v", err)
	}

	// Logout is idempotent.
	if err := uc.Logout(context.Background(), userID, "dev-1"); err != nil {
		t.Fatalf("second Logout failed: %v", err)
	}
}

func TestAuthLogin_RefreshStorageError(t *testing.T) {
	repo := newFakeAuthRepo()
	repo.saveHashErr = errors.InternalServer("REDIS_ERROR", "boom")
	uc := newTestAuthUsecase(repo)
	mustRegister(t, uc, "frank", "123456")

	if _, _, _, err := uc.Login(context.Background(), "frank", "123456", "dev-1"); err == nil {
		t.Fatalf("expected error when refresh storage fails")
	}
}
