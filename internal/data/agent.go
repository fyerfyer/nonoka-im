package data

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentRun struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	EventID      string `gorm:"uniqueIndex:idx_agent_run_event,priority:1;size:128;not null"`
	AgentID      int64  `gorm:"uniqueIndex:idx_agent_run_event,priority:2;index;not null"`
	AgentVersion int64  `gorm:"not null;default:0"`
	SessionID    uint64 `gorm:"index"`
	ActorUserID  int64  `gorm:"index;not null"`
	Topic        string `gorm:"index;size:128;not null"`
	MsgID        int64  `gorm:"index;not null"`
	TopicSeq     uint64 `gorm:"index;not null;default:0"`
	Status       string `gorm:"index;size:24;not null"`
	ErrorCode    string `gorm:"size:64"`
	Attempts     int    `gorm:"not null;default:0"`
	ReplyMsgID   int64
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CreatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

type AgentTurn struct {
	ID           uint64 `gorm:"primaryKey;autoIncrement"`
	AgentID      int64  `gorm:"index;not null"`
	ActorUserID  int64  `gorm:"index;not null"`
	Topic        string `gorm:"index;size:128;not null"`
	SessionID    uint64 `gorm:"index"`
	RunID        uint64 `gorm:"uniqueIndex;not null"`
	TriggerMsgID int64
	ReplyMsgID   int64
	Input        string `gorm:"type:text;not null"`
	Output       string `gorm:"type:text"`
	CreatedAt    time.Time
}

type AgentSession struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	AgentID     int64  `gorm:"index:idx_agent_session,unique;priority:1;not null"`
	ActorUserID int64  `gorm:"index:idx_agent_session,unique;priority:2;not null"`
	Topic       string `gorm:"index:idx_agent_session,unique;priority:3;size:128;not null"`
	UpdatedAt   time.Time
	CreatedAt   time.Time
}

type AgentContextSnapshot struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement"`
	SessionID     uint64 `gorm:"uniqueIndex;not null"`
	LastTurnID    uint64 `gorm:"not null"`
	AgentVersion  int64  `gorm:"not null"`
	Summary       string `gorm:"type:text;not null"`
	SummaryTokens int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Agent struct {
	ID               int64  `gorm:"primaryKey;autoIncrement"`
	BotUserID        int64  `gorm:"uniqueIndex;not null"`
	OwnerUserID      int64  `gorm:"index;not null"`
	Name             string `gorm:"size:100;not null"`
	Description      string `gorm:"type:text"`
	Status           string `gorm:"size:24;not null;default:draft"`
	CurrentVersion   int64
	DraftInstruction string `gorm:"type:text"`
	DraftModel       string `gorm:"size:100"`
	DraftTools       string `gorm:"type:text"`
	DraftBaseURL     string `gorm:"size:200"`
	DraftAPIKeyEnc   string `gorm:"type:text"`
	DraftMaxTokens   int    `gorm:"not null;default:4096"`
	DraftMCPBindings string `gorm:"type:text"`
	DraftSkills      string `gorm:"type:text"`
	DraftRevision    int64  `gorm:"not null;default:1"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AgentVersion struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	AgentID     int64  `gorm:"uniqueIndex:idx_agent_version,priority:1;not null"`
	Version     int64  `gorm:"uniqueIndex:idx_agent_version,priority:2;not null"`
	Instruction string `gorm:"type:text;not null"`
	Model       string `gorm:"size:100;not null"`
	Tools       string `gorm:"type:text;not null"`
	BaseURL     string `gorm:"size:200"`
	APIKeyEnc   string `gorm:"type:text"`
	MaxTokens   int    `gorm:"not null;default:4096"`
	MCPBindings string `gorm:"type:text"`
	Skills      string `gorm:"type:text"`
	CreatedAt   time.Time
}

type AgentStore struct{ db *gorm.DB }

func NewAgentStore(d *Data) *AgentStore { return &AgentStore{db: d.db} }

func sessionLockKey(agentID, actorID int64, topic string) int64 {
	key := fnv.New64a()
	_, _ = fmt.Fprintf(key, "agent-session:%d:%d:%s", agentID, actorID, topic)
	return int64(key.Sum64())
}

// PreparedRun is the outcome of PrepareRun. Claimed=false means the event was
// already processed or is in flight elsewhere; Queued=true means an earlier
// run in the session still blocks this one and the worker replay loop will
// pick it up later.
type PreparedRun struct {
	Session  *AgentSession
	Run      *AgentRun
	Claimed  bool
	Queued   bool
	Snapshot *AgentContextSnapshot
	Turns    []AgentTurn
}

// PrepareRun executes phase A of an agent run inside a short transaction that
// only holds the session advisory lock while it claims the run and loads the
// conversation state. The model call and reply submission must not run inside
// this transaction.
func (s *AgentStore) PrepareRun(ctx context.Context, agentID, actorID int64, topic string, candidate *AgentRun, topicSeq uint64) (*PreparedRun, error) {
	out := &PreparedRun{}
	var runErr error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", sessionLockKey(agentID, actorID, topic)).Error; err != nil {
			return err
		}
		locked := &AgentStore{db: tx}
		session, err := locked.GetSession(ctx, agentID, actorID, topic)
		if err != nil {
			return err
		}
		out.Session = session
		candidate.SessionID = session.ID
		run, claimed, err := locked.ClaimRun(ctx, candidate.EventID, candidate)
		if err != nil {
			return err
		}
		out.Run = run
		if !claimed {
			return nil
		}
		if topicSeq > 0 {
			blocker, err := locked.BlockingRun(ctx, session.ID, topicSeq, run.ID)
			if err != nil {
				return err
			}
			if blocker != nil {
				out.Queued = true
				return locked.UpdateRun(ctx, run.ID, map[string]interface{}{"status": "queued"})
			}
		}
		start := time.Now()
		if err := locked.UpdateRun(ctx, run.ID, map[string]interface{}{"status": "running", "started_at": start}); err != nil {
			return err
		}
		snapshot, err := locked.Snapshot(ctx, session.ID)
		if err != nil {
			runErr = err
			// Commit the failed status instead of rolling the run back to a
			// zombie "running" row.
			return locked.UpdateRun(ctx, run.ID, map[string]interface{}{"status": "failed", "error_code": classifyDataErr(err), "finished_at": time.Now()})
		}
		out.Snapshot = snapshot
		after := uint64(0)
		if snapshot != nil {
			after = snapshot.LastTurnID
		}
		turns, err := locked.TurnsAfter(ctx, session.ID, after, 1000)
		if err != nil {
			runErr = err
			return locked.UpdateRun(ctx, run.ID, map[string]interface{}{"status": "failed", "error_code": classifyDataErr(err), "finished_at": time.Now()})
		}
		out.Turns = turns
		out.Claimed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if runErr != nil {
		return nil, runErr
	}
	return out, nil
}

// CompleteRun executes phase C of an agent run: it re-acquires the session
// advisory lock in a short transaction and commits the turn, the context
// snapshot and the terminal run state together.
func (s *AgentStore) CompleteRun(ctx context.Context, agentID, actorID int64, topic string, turn *AgentTurn, snapshot *AgentContextSnapshot, runID uint64, values map[string]interface{}) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", sessionLockKey(agentID, actorID, topic)).Error; err != nil {
			return err
		}
		locked := &AgentStore{db: tx}
		if turn != nil {
			if err := locked.AddTurn(ctx, turn); err != nil {
				return err
			}
		}
		if snapshot != nil {
			if err := locked.SaveSnapshot(ctx, snapshot); err != nil {
				return err
			}
		}
		return locked.UpdateRun(ctx, runID, values)
	})
}

func classifyDataErr(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "run_failed"
}

func (s *AgentStore) GetSession(ctx context.Context, agentID, actorID int64, topic string) (*AgentSession, error) {
	row := AgentSession{AgentID: agentID, ActorUserID: actorID, Topic: topic}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "agent_id"}, {Name: "actor_user_id"}, {Name: "topic"}}, DoUpdates: clause.AssignmentColumns([]string{"updated_at"})}).Create(&row).Error
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).First(&row).Error
	return &row, err
}
func (s *AgentStore) ClaimRun(ctx context.Context, eventID string, run *AgentRun) (*AgentRun, bool, error) {
	var existing AgentRun
	where := s.db.WithContext(ctx).Where("event_id = ? AND agent_id = ?", eventID, run.AgentID)
	if err := where.First(&existing).Error; err == nil {
		staleRunning := existing.Status == "running" && ((existing.StartedAt != nil && existing.StartedAt.Before(time.Now().Add(-3*time.Minute))) || (existing.StartedAt == nil && existing.CreatedAt.Before(time.Now().Add(-3*time.Minute))))
		if existing.Status == "queued" || staleRunning {
			result := s.db.WithContext(ctx).Model(&AgentRun{}).Where("event_id = ? AND agent_id = ? AND status = ?", eventID, run.AgentID, "queued").Updates(map[string]interface{}{"status": "running", "attempts": gorm.Expr("attempts + 1"), "error_code": ""})
			if existing.Status == "running" {
				cutoff := time.Now().Add(-3 * time.Minute)
				result = s.db.WithContext(ctx).Model(&AgentRun{}).Where("event_id = ? AND agent_id = ? AND status = ? AND ((started_at IS NOT NULL AND started_at < ?) OR (started_at IS NULL AND created_at < ?))", eventID, run.AgentID, "running", cutoff, cutoff).Updates(map[string]interface{}{"attempts": gorm.Expr("attempts + 1"), "error_code": ""})
			}
			if result.Error != nil {
				return nil, false, result.Error
			}
			if result.RowsAffected == 1 {
				if err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("event_id = ? AND agent_id = ?", eventID, run.AgentID).First(&existing).Error; err != nil {
					return nil, false, err
				}
				return &existing, true, nil
			}
			if err := where.First(&existing).Error; err != nil {
				return nil, false, err
			}
		}
		return &existing, false, nil
	}
	if err := s.db.WithContext(ctx).Create(run).Error; err != nil {
		if err := where.First(&existing).Error; err == nil {
			return &existing, false, nil
		}
		return nil, false, err
	}
	return run, true, nil
}

func (s *AgentStore) BlockingRun(ctx context.Context, sessionID uint64, topicSeq uint64, runID uint64) (*AgentRun, error) {
	var row AgentRun
	err := s.db.WithContext(ctx).Where("session_id = ? AND id <> ? AND topic_seq < ? AND status IN ?", sessionID, runID, topicSeq, []string{"queued", "running"}).Order("topic_seq asc").First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &row, err
}

func (s *AgentStore) Snapshot(ctx context.Context, sessionID uint64) (*AgentContextSnapshot, error) {
	var row AgentContextSnapshot
	err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &row, err
}

func (s *AgentStore) SaveSnapshot(ctx context.Context, row *AgentContextSnapshot) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "session_id"}}, DoUpdates: clause.AssignmentColumns([]string{"last_turn_id", "agent_version", "summary", "summary_tokens", "updated_at"})}).Create(row).Error
}

func (s *AgentStore) TurnsAfter(ctx context.Context, sessionID, afterID uint64, limit int) ([]AgentTurn, error) {
	var rows []AgentTurn
	err := s.db.WithContext(ctx).Where("session_id = ? AND id > ?", sessionID, afterID).Order("id asc").Limit(limit).Find(&rows).Error
	return rows, err
}
func (s *AgentStore) UpdateRun(ctx context.Context, id uint64, values map[string]interface{}) error {
	return s.db.WithContext(ctx).Model(&AgentRun{}).Where("id = ?", id).Updates(values).Error
}
func (s *AgentStore) TouchSession(ctx context.Context, agentID, actorID int64, topic string) error {
	var row AgentSession
	err := s.db.WithContext(ctx).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).First(&row).Error
	if err == nil {
		return s.db.WithContext(ctx).Model(&row).Update("updated_at", time.Now()).Error
	}
	row = AgentSession{AgentID: agentID, ActorUserID: actorID, Topic: topic, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		// Concurrent event processing may have created the row; retry as update.
		return s.db.WithContext(ctx).Model(&AgentSession{}).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).Update("updated_at", time.Now()).Error
	}
	return nil
}
func (s *AgentStore) AddTurn(ctx context.Context, turn *AgentTurn) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "run_id"}}, DoNothing: true}).Create(turn).Error
}
func (s *AgentStore) RecentTurns(ctx context.Context, agentID, actorID int64, topic string, limit int) ([]AgentTurn, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []AgentTurn
	err := s.db.WithContext(ctx).Where("agent_id=? AND actor_user_id=? AND topic=?", agentID, actorID, topic).Order("created_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}
