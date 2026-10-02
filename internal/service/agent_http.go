package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"nonoka-im/internal/authz"
	"nonoka-im/internal/biz"
	"nonoka-im/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type AgentHTTP struct {
	store  *data.AgentStore
	auth   *authz.OpenFGA
	groups data.GroupRepo
	users  biz.AuthRepo
}

func NewAgentHTTP(d *data.Data, auth *authz.OpenFGA, groups data.GroupRepo) *AgentHTTP {
	return &AgentHTTP{store: data.NewAgentStore(d), auth: auth, groups: groups, users: data.NewAuthRepo(d, log.DefaultLogger)}
}

type agentRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Instruction string   `json:"instruction"`
	Model       string   `json:"model"`
	Tools       []string `json:"tools"`
	Revision    int64    `json:"revision"`
	Status      string   `json:"status"`
	Kind        string   `json:"kind"`
	Target      string   `json:"target"`
	Enabled     bool     `json:"enabled"`
}

type agentInputError struct{ error }

func invalidAgentInput(format string, args ...any) error {
	return agentInputError{fmt.Errorf(format, args...)}
}

func validateAgentRequest(v *agentRequest) error {
	v.Name = strings.TrimSpace(v.Name)
	v.Instruction = strings.TrimSpace(v.Instruction)
	if v.Model == "" {
		v.Model = os.Getenv("OPENAI_MODEL")
	}
	if v.Model == "" {
		v.Model = "deepseek-chat"
	}
	if len(v.Name) == 0 || len(v.Name) > 100 || v.Instruction == "" || len(v.Instruction) > 20000 || len(v.Description) > 4000 {
		return invalidAgentInput("invalid agent name, description or instruction")
	}
	if v.Model != "deepseek-chat" && v.Model != os.Getenv("OPENAI_MODEL") {
		return invalidAgentInput("unsupported model")
	}
	allowed := map[string]bool{"search_messages": true, "get_conversation_context": true, "send_message": true}
	seen := map[string]bool{}
	for _, name := range v.Tools {
		if !allowed[name] || seen[name] {
			return invalidAgentInput("unsupported or duplicate tool %q", name)
		}
		seen[name] = true
	}
	return nil
}

func parseAgentID(c khttp.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Vars().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, invalidAgentInput("invalid agent id")
	}
	return id, nil
}

func writeAgentJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func agentError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "AGENT_ERROR"
	var inputErr agentInputError
	switch {
	case errors.As(err, &inputErr):
		status, code = http.StatusBadRequest, "INVALID_ARGUMENT"
	case errors.Is(err, authz.ErrDenied):
		status, code = http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, data.ErrDraftConflict):
		status, code = http.StatusConflict, "DRAFT_CONFLICT"
	case errors.Is(err, data.ErrRunState):
		status, code = http.StatusConflict, "RUN_STATE"
	case errors.Is(err, data.ErrAgentNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	}
	writeAgentJSON(w, status, map[string]any{"code": code, "message": err.Error()})
}

func (s *AgentHTTP) handle(c khttp.Context, fn func(context.Context, http.ResponseWriter, *http.Request) error) error {
	h := c.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		if uid := extractUserIDFromContext(ctx); uid <= 0 {
			writeAgentJSON(c.Response(), http.StatusUnauthorized, map[string]string{"code": "UNAUTHORIZED", "message": "authentication required"})
			return nil, nil
		}
		if err := fn(ctx, c.Response(), c.Request()); err != nil {
			agentError(c.Response(), err)
		}
		return nil, nil
	})
	_, err := h(c, nil)
	return err
}

func decodeAgent(r *http.Request, dst *agentRequest) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return agentInputError{fmt.Errorf("invalid request: %w", err)}
	}
	return nil
}

func (s *AgentHTTP) List(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		uid := extractUserIDFromContext(ctx)
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil || limit <= 0 {
			limit = 100
		}
		if limit > 200 {
			limit = 200
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil || offset < 0 {
			offset = 0
		}
		rows, total, err := s.store.ListAgents(ctx, limit, offset)
		if err != nil {
			return err
		}
		visible := make([]map[string]any, 0)
		for _, row := range rows {
			if s.auth.CanViewAgent(ctx, uid, row.ID) == nil {
				visible = append(visible, agentView(&row, row.OwnerUserID == uid))
			}
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"agents": visible, "total": total})
		return nil
	})
}

func agentView(row *data.Agent, manager bool) map[string]any {
	view := map[string]any{"id": row.ID, "botUserId": row.BotUserID, "ownerUserId": row.OwnerUserID, "name": row.Name, "description": row.Description, "status": row.Status, "currentVersion": row.CurrentVersion, "canManage": manager}
	if manager {
		view["draft"] = map[string]any{"instruction": row.DraftInstruction, "model": row.DraftModel, "tools": json.RawMessage(row.DraftTools), "revision": row.DraftRevision}
	}
	return view
}

func (s *AgentHTTP) Detail(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		uid := extractUserIDFromContext(ctx)
		if err := s.auth.CanViewAgent(ctx, uid, id); err != nil {
			return authz.ErrDenied
		}
		row, err := s.store.GetAgent(ctx, id)
		if err != nil {
			return data.ErrAgentNotFound
		}
		manager := s.auth.CanManageAgent(ctx, uid, id) == nil
		view := agentView(row, manager)
		if row.CurrentVersion > 0 {
			version, err := s.store.GetVersion(ctx, id, row.CurrentVersion)
			if err != nil {
				return err
			}
			view["published"] = version
		}
		writeAgentJSON(w, http.StatusOK, view)
		return nil
	})
}

func (s *AgentHTTP) Create(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		if err := validateAgentRequest(&input); err != nil {
			return err
		}
		tools, _ := json.Marshal(input.Tools)
		uid := extractUserIDFromContext(ctx)
		row, err := s.store.CreateAgent(ctx, uid, input.Name, input.Description, input.Instruction, input.Model, string(tools), func(ctx context.Context, id int64) error { return s.auth.SetAgentOwner(ctx, id, uid) })
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusCreated, agentView(row, true))
		return nil
	})
}

func (s *AgentHTTP) Draft(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.auth.CanManageAgent(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return authz.ErrDenied
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		if err := validateAgentRequest(&input); err != nil {
			return err
		}
		tools, _ := json.Marshal(input.Tools)
		row, err := s.store.UpdateDraft(ctx, id, input.Revision, input.Name, input.Description, input.Instruction, input.Model, string(tools))
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, agentView(row, true))
		return nil
	})
}

func (s *AgentHTTP) Publish(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.auth.CanManageAgent(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return authz.ErrDenied
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		version, err := s.store.Publish(ctx, id, input.Revision)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, version)
		return nil
	})
}

func (s *AgentHTTP) Status(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.auth.CanManageAgent(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return authz.ErrDenied
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		if input.Status != "active" && input.Status != "disabled" {
			return invalidAgentInput("invalid agent status")
		}
		if err := s.store.SetAgentStatus(ctx, id, input.Status); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": input.Status})
		return nil
	})
}

func (s *AgentHTTP) Share(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.auth.CanManageAgent(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return authz.ErrDenied
		}
		var input agentRequest
		if err := decodeAgent(r, &input); err != nil {
			return err
		}
		if input.Kind != "user" && input.Kind != "group" {
			return invalidAgentInput("invalid share kind")
		}
		if input.Kind == "user" {
			uid, err := strconv.ParseInt(input.Target, 10, 64)
			if err != nil || uid <= 0 {
				return invalidAgentInput("invalid target user")
			}
			_, err = s.users.GetUserByID(ctx, uid)
			if kerrors.IsNotFound(err) {
				return invalidAgentInput("target user does not exist")
			}
			if err != nil {
				return err
			}
		}
		row, err := s.store.GetAgent(ctx, id)
		if err != nil {
			return data.ErrAgentNotFound
		}
		if input.Kind == "group" {
			if err := s.auth.CanManageGroup(ctx, extractUserIDFromContext(ctx), input.Target); err != nil {
				return authz.ErrDenied
			}
			if input.Enabled {
				// GroupRepo.AddMember tolerates duplicate keys, so re-sharing
				// to an already-enrolled bot stays idempotent.
				if err := s.groups.AddMember(ctx, input.Target, row.BotUserID); err != nil {
					return err
				}
				if err := s.auth.SetGroupMember(ctx, input.Target, row.BotUserID, true); err != nil {
					return err
				}
			}
		}
		if err := s.auth.SetAgentShare(ctx, id, input.Kind, input.Target, input.Enabled); err != nil {
			return err
		}
		if input.Kind == "group" && !input.Enabled {
			// Revocation only removes the authorization tuple; the bot stays
			// a member of the group conversation itself.
			if err := s.auth.SetGroupMember(ctx, input.Target, row.BotUserID, false); err != nil {
				return err
			}
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"kind": input.Kind, "target": input.Target, "enabled": input.Enabled})
		return nil
	})
}

func (s *AgentHTTP) Shares(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.auth.CanManageAgent(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return authz.ErrDenied
		}
		shares, err := s.auth.ListAgentShares(ctx, id)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"shares": shares})
		return nil
	})
}

func (s *AgentHTTP) Runs(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.auth.CanManageAgent(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return authz.ErrDenied
		}
		runs, err := s.store.ListRuns(ctx, id, 50)
		if err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]any{"runs": runs})
		return nil
	})
}

func (s *AgentHTTP) RunDetail(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid run id")
		}
		run, err := s.store.GetRun(ctx, id)
		if err != nil {
			return data.ErrAgentNotFound
		}
		uid := extractUserIDFromContext(ctx)
		if uid != run.ActorUserID && s.auth.CanManageAgent(ctx, uid, run.AgentID) != nil {
			return authz.ErrDenied
		}
		writeAgentJSON(w, http.StatusOK, run)
		return nil
	})
}

func (s *AgentHTTP) RunAction(c khttp.Context, action string) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid run id")
		}
		run, err := s.store.GetRun(ctx, id)
		if err != nil {
			return data.ErrAgentNotFound
		}
		uid := extractUserIDFromContext(ctx)
		if uid != run.ActorUserID && s.auth.CanManageAgent(ctx, uid, run.AgentID) != nil {
			return authz.ErrDenied
		}
		if err := s.store.SetRunAction(ctx, id, action); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"action": action})
		return nil
	})
}

func (s *AgentHTTP) VerifyBotSend(ctx context.Context, claims map[string]any) error {
	agentID, ok1 := claims["agent_id"].(float64)
	botID, ok2 := claims["user_id"].(float64)
	rawTopic, ok3 := claims["topic"].(string)
	if !ok1 || !ok2 || !ok3 || claims["scope"] != "agent:send" {
		return authz.ErrDenied
	}
	row, err := s.store.GetAgent(ctx, int64(agentID))
	if err != nil || row.BotUserID != int64(botID) || row.CurrentVersion <= 0 {
		return authz.ErrDenied
	}
	if err := s.auth.CanAccessTopic(ctx, int64(botID), rawTopic); err != nil {
		return authz.ErrDenied
	}
	return nil
}
