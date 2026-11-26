package botdata

import "time"

type userRow struct {
	ID       int64  `gorm:"column:id;primaryKey"`
	Username string `gorm:"column:username"`
	Password string `gorm:"column:password"`
	IsBot    bool   `gorm:"column:is_bot"`
}

func (userRow) TableName() string { return tableUsers }

type botRow struct {
	ID                 int64     `gorm:"column:id;primaryKey"`
	BotUserID          int64     `gorm:"column:bot_user_id"`
	OwnerUserID        int64     `gorm:"column:owner_user_id"`
	Username           string    `gorm:"column:username;->"`
	Name               string    `gorm:"column:name"`
	Description        string    `gorm:"column:description"`
	Status             string    `gorm:"column:status"`
	EventSubscriptions string    `gorm:"column:event_subscriptions"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (botRow) TableName() string { return tableBots }

type tokenRow struct {
	ID         string     `gorm:"column:id;primaryKey"`
	BotID      int64      `gorm:"column:bot_id"`
	Name       string     `gorm:"column:name"`
	Prefix     string     `gorm:"column:prefix"`
	TokenHash  []byte     `gorm:"column:token_hash"`
	Scopes     string     `gorm:"column:scopes"`
	ExpiresAt  *time.Time `gorm:"column:expires_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
	LastUsedAt *time.Time `gorm:"column:last_used_at"`
	CreatedBy  int64      `gorm:"column:created_by"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

func (tokenRow) TableName() string { return tableTokens }

type webhookRow struct {
	ID                  int64      `gorm:"column:id;primaryKey"`
	BotID               int64      `gorm:"column:bot_id"`
	URL                 string     `gorm:"column:url"`
	SecretCiphertext    []byte     `gorm:"column:secret_ciphertext"`
	SecretPrefix        string     `gorm:"column:secret_prefix"`
	Events              string     `gorm:"column:events"`
	Enabled             bool       `gorm:"column:enabled"`
	VerifiedAt          *time.Time `gorm:"column:verified_at"`
	LastSuccessAt       *time.Time `gorm:"column:last_success_at"`
	ConsecutiveFailures int        `gorm:"column:consecutive_failures"`
	DisabledAt          *time.Time `gorm:"column:disabled_at"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

func (webhookRow) TableName() string { return tableWebhooks }

type deliveryRow struct {
	ID              int64      `gorm:"column:id;primaryKey"`
	BotID           int64      `gorm:"column:bot_id"`
	EventID         string     `gorm:"column:event_id"`
	EventType       string     `gorm:"column:event_type"`
	Topic           string     `gorm:"column:topic"`
	MsgID           int64      `gorm:"column:msg_id"`
	Status          string     `gorm:"column:status"`
	Attempts        int        `gorm:"column:attempts"`
	NextAttemptAt   time.Time  `gorm:"column:next_attempt_at"`
	LockedUntil     *time.Time `gorm:"column:locked_until"`
	LastStatusCode  *int       `gorm:"column:last_status_code"`
	LastError       string     `gorm:"column:last_error"`
	ResponseSnippet string     `gorm:"column:response_snippet"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	DeliveredAt     *time.Time `gorm:"column:delivered_at"`
}

func (deliveryRow) TableName() string { return tableDeliveries }
