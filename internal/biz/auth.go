package biz

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"time"

	"nonoka-im/internal/conf"

	"github.com/go-kratos/kratos/v2/errors"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        int64
	Username  string
	Password  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AuthRepo interface {
	CreateUser(ctx context.Context, u *User) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)
	SearchUsersByPrefix(ctx context.Context, prefix string, limit int32) ([]*User, error)
	// Refresh tokens are opaque random strings; only their SHA-256 hash is
	// stored in Redis under im:refresh:{user_id}:{device_id} with a TTL.
	SaveRefreshTokenHash(ctx context.Context, userID int64, deviceID, hash string, ttl time.Duration) error
	// GetRefreshTokenHash returns the stored hash; errors.Is(err,
	// errors.IsNotFound) reports a missing/expired token.
	GetRefreshTokenHash(ctx context.Context, userID int64, deviceID string) (string, error)
	DeleteRefreshToken(ctx context.Context, userID int64, deviceID string) error
}

type AuthUsecase struct {
	repo      AuthRepo
	jwtSecret []byte
	tokenTTL  time.Duration
	// refreshTTL is how long a refresh token stays valid (rotation window).
	refreshTTL time.Duration
	// bcryptCost is the hashing cost for new passwords (bcrypt.DefaultCost
	// unless overridden; tests lower it to reduce CPU load).
	bcryptCost int
}

// Repo returns the underlying AuthRepo for user queries.
func (uc *AuthUsecase) Repo() AuthRepo {
	return uc.repo
}

func NewAuthUsecase(repo AuthRepo, authConf *conf.Auth) *AuthUsecase {
	ttl := 7 * 24 * time.Hour
	refreshTTL := 30 * 24 * time.Hour
	if authConf.TokenTtl != nil {
		ttl = authConf.TokenTtl.AsDuration()
	}
	if authConf.RefreshTokenTtl != nil {
		refreshTTL = authConf.RefreshTokenTtl.AsDuration()
	}
	bcryptCost := bcrypt.DefaultCost
	if c := int(authConf.BcryptCost); c >= bcrypt.MinCost && c <= bcrypt.MaxCost {
		bcryptCost = c
	}
	return &AuthUsecase{
		repo:       repo,
		jwtSecret:  []byte(authConf.JwtSecret),
		tokenTTL:   ttl,
		refreshTTL: refreshTTL,
		bcryptCost: bcryptCost,
	}
}

func (uc *AuthUsecase) Register(ctx context.Context, username, password string) (*User, error) {
	// Pre-check if username already exists to return a friendly error
	existing, err := uc.repo.GetUserByUsername(ctx, username)
	if err != nil && !errors.Is(err, errors.NotFound("USER_NOT_FOUND", "user not found")) {
		return nil, err
	}
	if existing != nil {
		return nil, errors.BadRequest("USERNAME_EXISTS", "username already exists")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), uc.bcryptCost)
	if err != nil {
		return nil, err
	}

	user := &User{
		Username: username,
		Password: string(hashedPassword),
	}
	return uc.repo.CreateUser(ctx, user)
}

// generateRefreshToken returns a new opaque refresh token (32 random bytes
// as hex) and the SHA-256 hex hash that is persisted in Redis. Only the
// hash is stored so a leaked Redis value cannot be replayed directly.
func generateRefreshToken() (token, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(sum[:])
	return token, hash, nil
}

func (uc *AuthUsecase) signAccessToken(user *User, deviceID string) (string, error) {
	now := time.Now()
	claims := jwt5.MapClaims{
		"user_id":   user.ID,
		"username":  user.Username,
		"device_id": deviceID,
		"iat":       now.Unix(),
		"exp":       now.Add(uc.tokenTTL).Unix(),
	}

	token := jwt5.NewWithClaims(jwt5.SigningMethodHS256, claims)
	return token.SignedString(uc.jwtSecret)
}

// issueTokenPair signs a new access token and persists a rotated refresh
// token for the user's device, replacing any previous one.
func (uc *AuthUsecase) issueTokenPair(ctx context.Context, user *User, deviceID string) (accessToken, refreshToken string, err error) {
	accessToken, err = uc.signAccessToken(user, deviceID)
	if err != nil {
		return "", "", err
	}

	refreshToken, hash, err := generateRefreshToken()
	if err != nil {
		return "", "", err
	}
	if err := uc.repo.SaveRefreshTokenHash(ctx, user.ID, deviceID, hash, uc.refreshTTL); err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func (uc *AuthUsecase) Login(ctx context.Context, username, password, deviceID string) (int64, string, string, error) {
	user, err := uc.repo.GetUserByUsername(ctx, username)
	if err != nil {
		// Return vague error to prevent user enumeration attacks
		return 0, "", "", errors.Unauthorized("AUTH_FAILED", "invalid username or password")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return 0, "", "", errors.Unauthorized("AUTH_FAILED", "invalid username or password")
	}

	accessToken, refreshToken, err := uc.issueTokenPair(ctx, user, deviceID)
	if err != nil {
		return 0, "", "", err
	}
	return user.ID, accessToken, refreshToken, nil
}

// Refresh validates a presented refresh token against the stored hash and,
// on success, rotates the pair: the old refresh token is deleted and a new
// access/refresh token pair is issued for the same device.
func (uc *AuthUsecase) Refresh(ctx context.Context, userID int64, deviceID, refreshToken string) (int64, string, string, error) {
	if refreshToken == "" {
		return 0, "", "", errors.Unauthorized("INVALID_REFRESH_TOKEN", "refresh token required")
	}

	storedHash, err := uc.repo.GetRefreshTokenHash(ctx, userID, deviceID)
	if err != nil {
		// Missing or expired token: same vague error to avoid probing.
		return 0, "", "", errors.Unauthorized("INVALID_REFRESH_TOKEN", "invalid or expired refresh token")
	}
	sum := sha256.Sum256([]byte(refreshToken))
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(hex.EncodeToString(sum[:]))) != 1 {
		return 0, "", "", errors.Unauthorized("INVALID_REFRESH_TOKEN", "invalid or expired refresh token")
	}

	user, err := uc.repo.GetUserByID(ctx, userID)
	if err != nil {
		return 0, "", "", err
	}

	// Rotate: delete the old token before issuing the new pair so a
	// replayed refresh token can never be valid twice.
	if err := uc.repo.DeleteRefreshToken(ctx, userID, deviceID); err != nil {
		return 0, "", "", err
	}
	accessToken, newRefreshToken, err := uc.issueTokenPair(ctx, user, deviceID)
	if err != nil {
		return 0, "", "", err
	}
	return user.ID, accessToken, newRefreshToken, nil
}

// Logout revokes the refresh token of a device. It is idempotent: revoking
// a token that does not exist is not an error.
func (uc *AuthUsecase) Logout(ctx context.Context, userID int64, deviceID string) error {
	return uc.repo.DeleteRefreshToken(ctx, userID, deviceID)
}