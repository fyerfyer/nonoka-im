package biz

import (
	"context"
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
}

type AuthUsecase struct {
	repo      AuthRepo
	jwtSecret []byte
	tokenTTL  time.Duration
}

func NewAuthUsecase(repo AuthRepo, authConf *conf.Auth) *AuthUsecase {
	ttl := 7 * 24 * time.Hour
	if authConf.TokenTtl != nil {
		ttl = authConf.TokenTtl.AsDuration()
	}
	return &AuthUsecase{
		repo:      repo,
		jwtSecret: []byte(authConf.JwtSecret),
		tokenTTL:  ttl,
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

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{
		Username: username,
		Password: string(hashedPassword),
	}
	return uc.repo.CreateUser(ctx, user)
}

func (uc *AuthUsecase) Login(ctx context.Context, username, password, deviceID string) (int64, string, error) {
	user, err := uc.repo.GetUserByUsername(ctx, username)
	if err != nil {
		// Return vague error to prevent user enumeration attacks
		return 0, "", errors.Unauthorized("AUTH_FAILED", "invalid username or password")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return 0, "", errors.Unauthorized("AUTH_FAILED", "invalid username or password")
	}

	now := time.Now()
	claims := jwt5.MapClaims{
		"user_id":   user.ID,
		"username":  user.Username,
		"device_id": deviceID,
		"iat":       now.Unix(),
		"exp":       now.Add(uc.tokenTTL).Unix(),
	}

	token := jwt5.NewWithClaims(jwt5.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(uc.jwtSecret)
	if err != nil {
		return 0, "", err
	}
	return user.ID, tokenString, nil
}