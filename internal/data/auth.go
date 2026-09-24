package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nonoka-im/internal/biz"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type User struct {
	ID       int64  `gorm:"primaryKey;autoIncrement"`
	Username string `gorm:"uniqueIndex;type:varchar(50);not null"`
	Password string `gorm:"type:varchar(255);not null"`
}

type authRepo struct {
	data *Data
	log  *log.Helper
}

func NewAuthRepo(data *Data, logger log.Logger) biz.AuthRepo {
	return &authRepo{
		data: data,
		log:  log.NewHelper(logger),
	}
}

func (r *authRepo) CreateUser(ctx context.Context, u *biz.User) (*biz.User, error) {
	dbUser := &User{
		Username: u.Username,
		Password: u.Password,
	}

	result := r.data.db.WithContext(ctx).Create(dbUser)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) || strings.Contains(result.Error.Error(), "duplicate key") {
			return nil, errors.BadRequest("USERNAME_EXISTS", "username already exists")
		}
		r.log.Errorf("CreateUser failed: %v", result.Error)
		return nil, errors.InternalServer("DB_ERROR", "database error")
	}
	u.ID = dbUser.ID
	return u, nil
}

func (r *authRepo) GetUserByUsername(ctx context.Context, username string) (*biz.User, error) {
	var dbUser User
	result := r.data.db.WithContext(ctx).Where("username = ?", username).First(&dbUser)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.NotFound("USER_NOT_FOUND", "user not found")
		}
		r.log.Errorf("GetUserByUsername failed: %v", result.Error)
		return nil, errors.InternalServer("DB_ERROR", "database error")
	}

	return &biz.User{
		ID:       dbUser.ID,
		Username: dbUser.Username,
		Password: dbUser.Password,
	}, nil
}

func (r *authRepo) GetUserByID(ctx context.Context, id int64) (*biz.User, error) {
	var dbUser User
	result := r.data.db.WithContext(ctx).First(&dbUser, id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.NotFound("USER_NOT_FOUND", "user not found")
		}
		r.log.Errorf("GetUserByID failed: %v", result.Error)
		return nil, errors.InternalServer("DB_ERROR", "database error")
	}

	return &biz.User{
		ID:       dbUser.ID,
		Username: dbUser.Username,
		Password: dbUser.Password,
	}, nil
}

// refreshTokenKey returns the Redis key holding the refresh token hash
// for a user's device.
func refreshTokenKey(userID int64, deviceID string) string {
	return fmt.Sprintf("im:refresh:%d:%s", userID, deviceID)
}

func (r *authRepo) SaveRefreshTokenHash(ctx context.Context, userID int64, deviceID, hash string, ttl time.Duration) error {
	if r.data.Redis == nil {
		return errors.InternalServer("REDIS_UNAVAILABLE", "redis is not configured")
	}
	return r.data.Redis.Set(ctx, refreshTokenKey(userID, deviceID), hash, ttl).Err()
}

func (r *authRepo) GetRefreshTokenHash(ctx context.Context, userID int64, deviceID string) (string, error) {
	if r.data.Redis == nil {
		return "", errors.InternalServer("REDIS_UNAVAILABLE", "redis is not configured")
	}
	hash, err := r.data.Redis.Get(ctx, refreshTokenKey(userID, deviceID)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", errors.NotFound("REFRESH_TOKEN_NOT_FOUND", "refresh token not found or expired")
		}
		r.log.Errorf("GetRefreshTokenHash failed: %v", err)
		return "", errors.InternalServer("REDIS_ERROR", "redis error")
	}
	return hash, nil
}

func (r *authRepo) DeleteRefreshToken(ctx context.Context, userID int64, deviceID string) error {
	if r.data.Redis == nil {
		return errors.InternalServer("REDIS_UNAVAILABLE", "redis is not configured")
	}
	return r.data.Redis.Del(ctx, refreshTokenKey(userID, deviceID)).Err()
}

func (r *authRepo) SearchUsersByPrefix(ctx context.Context, prefix string, limit int32) ([]*biz.User, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return []*biz.User{}, nil
	}

	var rows []User
	result := r.data.db.WithContext(ctx).
		Where("username LIKE ?", prefix+"%").
		Order("username ASC").
		Limit(int(limit)).
		Find(&rows)
	if result.Error != nil {
		r.log.Errorf("SearchUsersByPrefix failed: %v", result.Error)
		return nil, errors.InternalServer("DB_ERROR", "database error")
	}

	users := make([]*biz.User, len(rows))
	for i := range rows {
		users[i] = &biz.User{
			ID:       rows[i].ID,
			Username: rows[i].Username,
			Password: rows[i].Password,
		}
	}
	return users, nil
}