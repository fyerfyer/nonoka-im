package data

import (
	"context"
	"strings"

	"nonoka-im/internal/biz"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
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