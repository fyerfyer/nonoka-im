package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/agent/credential"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/tools"
	"nonoka-im/internal/agent/tools/catalog"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/biz"
	"nonoka-im/internal/data"
	agentdata "nonoka-im/internal/data/agent"
	imdata "nonoka-im/internal/data/im"
	platformdata "nonoka-im/internal/data/platform"
	"nonoka-im/internal/httpapi"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

type AgentHTTP struct {
	store  *agentdata.Store
	ext    *platformdata.Store
	auth   *authz.OpenFGA
	groups imdata.GroupRepo
	users  biz.AuthRepo
	idem   *httpapi.Idempotency
}

func NewAgentHTTP(d *data.Data, auth *authz.OpenFGA, groups imdata.GroupRepo) *AgentHTTP {
	return &AgentHTTP{
		store: agentdata.NewStore(d.DB()), ext: platformdata.NewStore(d.DB()),
		auth: auth, groups: groups, users: imdata.NewAuthRepo(d.DB(), d.Redis, log.DefaultLogger),
		idem: &httpapi.Idempotency{Store: platformdata.NewIdempotencyStore(d.DB()), LockTTL: 30 * time.Second},
	}
}

type agentRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Instruction string   `json:"instruction"`
	Model       string   `json:"model"`
	Tools       []string `json:"tools"`
	BaseURL     string   `json:"base_url"`
	APIKey      string   `json:"api_key"`
	MaxTokens   int      `json:"max_tokens"`
	MCPBindings []struct {
		Server string `json:"server"`
		Tool   string `json:"tool"`
	} `json:"mcp_bindings"`
	Skills []struct {
		Name    string `json:"name"`
		Version int64  `json:"version"`
	} `json:"skills"`
	Revision int64  `json:"revision"`
	Status   string `json:"status"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Enabled  bool   `json:"enabled"`
}

type agentInputError struct{ error }

func invalidAgentInput(format string, args ...any) error {
	return agentInputError{fmt.Errorf(format, args...)}
}

// validateExtensions checks MCP and skill bindings against the registries and
// returns their frozen JSON representation for the draft/version snapshot.
func (s *AgentHTTP) validateExtensions(ctx context.Context, v *agentRequest) (string, string, error) {
	mcpBindings := make([]domain.MCPBinding, 0, len(v.MCPBindings))
	seen := map[string]bool{}
	for _, binding := range v.MCPBindings {
		if binding.Server == "" || binding.Tool == "" || strings.ContainsAny(binding.Server+binding.Tool, " \t\n") {
			return "", "", invalidAgentInput("invalid mcp binding")
		}
		key := binding.Server + "/" + binding.Tool
		if seen[key] {
			return "", "", invalidAgentInput("duplicate mcp binding %q", key)
		}
		seen[key] = true
		server, err := s.ext.GetServer(ctx, binding.Server)
		if err != nil || !server.Enabled {
			return "", "", invalidAgentInput("mcp server %q is not available", binding.Server)
		}
		var allowed []string
		if err := json.Unmarshal([]byte(server.AllowedTools), &allowed); err != nil {
			return "", "", invalidAgentInput("mcp server %q has an invalid tool list", binding.Server)
		}
		found := false
		for _, name := range allowed {
			if name == binding.Tool {
				found = true
				break
			}
		}
		if !found {
			return "", "", invalidAgentInput("tool %q is not allowed on mcp server %q", binding.Tool, binding.Server)
		}
		mcpBindings = append(mcpBindings, domain.MCPBinding{
			Server: binding.Server, Tool: binding.Tool,
			Exposed: tools.ExposedMCPName(binding.Server, binding.Tool),
		})
	}
	skillBindings := make([]domain.SkillBinding, 0, len(v.Skills))
	seenSkill := map[string]bool{}
	for _, binding := range v.Skills {
		if binding.Name == "" || binding.Version <= 0 {
			return "", "", invalidAgentInput("invalid skill binding")
		}
		if seenSkill[binding.Name] {
			return "", "", invalidAgentInput("duplicate skill binding %q", binding.Name)
		}
		seenSkill[binding.Name] = true
		row, err := s.ext.GetSkill(ctx, binding.Name, binding.Version)
		if err != nil || !row.Published {
			return "", "", invalidAgentInput("skill %q version %d is not published", binding.Name, binding.Version)
		}
		skillBindings = append(skillBindings, domain.SkillBinding{Name: binding.Name, Version: binding.Version})
	}
	mcpJSON, _ := json.Marshal(mcpBindings)
	skillsJSON, _ := json.Marshal(skillBindings)
	return string(mcpJSON), string(skillsJSON), nil
}

// isAgentAdmin reports whether the user is in the platform admin set
// configured through AGENT_ADMIN_USER_IDS (comma separated user IDs).
func isAgentAdmin(uid int64) bool {
	for _, part := range strings.Split(os.Getenv("AGENT_ADMIN_USER_IDS"), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id == uid {
			return true
		}
	}
	return false
}

func validateAgentRequest(v *agentRequest) error {
	v.Name = strings.TrimSpace(v.Name)
	v.Instruction = strings.TrimSpace(v.Instruction)
	v.Model = strings.TrimSpace(v.Model)
	if v.Model == "" {
		v.Model = os.Getenv("OPENAI_MODEL")
	}
	if len(v.Name) == 0 || len(v.Name) > 100 || v.Instruction == "" || len(v.Instruction) > 20000 || len(v.Description) > 4000 {
		return invalidAgentInput("invalid agent name, description or instruction")
	}
	if len(v.Model) > 100 {
		return invalidAgentInput("invalid model")
	}
	v.BaseURL = strings.TrimSpace(v.BaseURL)
	if v.BaseURL != "" {
		if err := validateModelBaseURL(v.BaseURL); err != nil {
			return invalidAgentInput("%v", err)
		}
	}
	if len(v.APIKey) > 512 {
		return invalidAgentInput("invalid api_key")
	}
	if v.MaxTokens < 0 || v.MaxTokens > 128000 {
		return invalidAgentInput("invalid max_tokens")
	}
	allowed := make(map[string]bool)
	for _, def := range catalog.BuiltinDefinitions() {
		allowed[def.Name] = true
	}
	seen := map[string]bool{}
	for _, name := range v.Tools {
		if !allowed[name] || seen[name] {
			return invalidAgentInput("unsupported or duplicate tool %q", name)
		}
		seen[name] = true
	}
	return nil
}

// sealAgentKey encrypts a caller-supplied model API key. An empty key means
// "keep the existing one" and yields updateKey=false.
func sealAgentKey(apiKey string) (enc string, updateKey bool, err error) {
	if apiKey == "" {
		return "", false, nil
	}
	key, err := credential.SealKeyFromEnv()
	if err != nil {
		return "", false, err
	}
	sealed, err := credential.Seal(key, []byte(apiKey))
	if err != nil {
		return "", false, err
	}
	return base64.StdEncoding.EncodeToString(sealed), true, nil
}

// validateModelBaseURL is the transitional SSRF guard for agent-owned model
// endpoints. Shared agents must eventually use the platform Model Service; in
// the meantime this rejects private/link-local targets and enforces HTTPS plus
// an optional host allowlist (AGENT_MODEL_ALLOWED_HOSTS).
func validateModelBaseURL(raw string) error {
	if len(raw) > 200 {
		return fmt.Errorf("invalid base_url")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("base_url must be an http(s) URL")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("base_url must include a host")
	}
	if u.Scheme == "http" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("base_url must use https")
	}
	if allow := strings.TrimSpace(os.Getenv("AGENT_MODEL_ALLOWED_HOSTS")); allow != "" {
		allowed := false
		for _, entry := range strings.Split(allow, ",") {
			entry = strings.ToLower(strings.TrimSpace(entry))
			if entry == "" {
				continue
			}
			if host == entry || strings.HasSuffix(host, "."+entry) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("base_url host %q is not allowed", host)
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("base_url must not target a private or loopback address")
		}
		return nil
	}
	// Resolve hostnames so a public-looking name cannot point at internal
	// metadata or a private network. Resolution failures are rejected: a host
	// that cannot be resolved is not a usable model endpoint.
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("base_url host %q could not be resolved", host)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("base_url must not target a private or loopback address")
		}
	}
	return nil
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast()
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

func agentHTTPError(err error) *httpapi.Error {
	var inputErr agentInputError
	switch {
	case errors.As(err, &inputErr):
		return httpapi.BadRequest("INVALID_ARGUMENT", err.Error())
	case errors.Is(err, authz.ErrDenied):
		return httpapi.Forbidden("access denied")
	case errors.Is(err, domain.ErrDraftConflict):
		return httpapi.Conflict("DRAFT_CONFLICT", err.Error())
	case errors.Is(err, domain.ErrRunState):
		return httpapi.Conflict("RUN_STATE", err.Error())
	case errors.Is(err, domain.ErrApprovalExpired):
		return httpapi.Conflict("APPROVAL_EXPIRED", err.Error())
	case errors.Is(err, domain.ErrApprovalGone):
		return httpapi.Conflict("APPROVAL_NOT_PENDING", err.Error())
	case errors.Is(err, domain.ErrSideEffectCommitted):
		return httpapi.Conflict("SIDE_EFFECT_COMMITTED", "run already committed a side effect; retry requires acknowledgeSideEffect=true")
	case errors.Is(err, domain.ErrMCPConflict):
		return httpapi.Conflict("MCP_EXISTS", err.Error())
	case errors.Is(err, domain.ErrAgentNotFound), errors.Is(err, domain.ErrVersionNotFound),
		errors.Is(err, domain.ErrRunNotFound), errors.Is(err, domain.ErrApprovalNotFound),
		errors.Is(err, domain.ErrSkillNotFound):
		return httpapi.NotFound(err.Error())
	default:
		return httpapi.Internal(err)
	}
}

func (s *AgentHTTP) handle(c khttp.Context, fn func(context.Context, http.ResponseWriter, *http.Request) error) error {
	h := c.Middleware(func(ctx context.Context, _ interface{}) (interface{}, error) {
		if uid := extractUserIDFromContext(ctx); uid <= 0 {
			httpapi.Write(c.Response(), ctx, &httpapi.Error{Status: http.StatusUnauthorized, Code: "UNAUTHORIZED", Message: "authentication required"})
			return nil, nil
		}
		uid := extractUserIDFromContext(ctx)
		s.idem.Execute(ctx, c.Response(), c.Request(), uid, func(w http.ResponseWriter) error {
			if err := fn(ctx, w, c.Request()); err != nil {
				return agentHTTPError(err)
			}
			return nil
		})
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
		limit := httpapi.Limit(r, 50, 200)
		offset, err := httpapi.Offset(r.URL.Query().Get("cursor"))
		if err != nil {
			return invalidAgentInput("invalid cursor")
		}
		// Visibility is decided by OpenFGA after the DB read, so scan until the
		// page is full or the table is exhausted. Returning a raw hasMore flag
		// here would advertise invisible rows and produce empty pages.
		visible := make([]AgentView, 0, limit)
		scanned := offset
		for len(visible) < limit {
			rows, total, err := s.store.ListAgents(ctx, limit-len(visible), scanned)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				scanned = int(total)
				break
			}
			for i := range rows {
				scanned++
				if s.auth.CanViewAgent(ctx, uid, rows[i].ID) == nil {
					visible = append(visible, agentView(&rows[i], rows[i].OwnerUserID == uid))
					if len(visible) == limit {
						break
					}
				}
			}
			if int64(scanned) >= total {
				break
			}
		}
		var total int64
		if _, total, err = s.store.ListAgents(ctx, 1, 0); err != nil {
			return err
		}
		page := httpapi.Page[AgentView]{Items: visible, HasMore: int64(scanned) < total}
		if page.HasMore {
			page.NextCursor = httpapi.EncodeCursor(scanned)
		}
		writeAgentJSON(w, http.StatusOK, page)
		return nil
	})
}

// rawJSON renders an empty snapshot column as an empty JSON array instead of
// an invalid empty RawMessage.
func rawJSON(s string) json.RawMessage {
	if strings.TrimSpace(s) == "" {
		return json.RawMessage("[]")
	}
	return json.RawMessage(s)
}

func (s *AgentHTTP) Detail(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		uid := extractUserIDFromContext(ctx)
		row, err := s.store.GetAgent(ctx, id)
		if err != nil {
			return domain.ErrAgentNotFound
		}
		if err := s.auth.CanViewAgent(ctx, uid, id); err != nil {
			return authz.ErrDenied
		}
		manager := s.auth.CanManageAgent(ctx, uid, id) == nil
		view := agentView(row, manager)
		// Only a manager may read the published instruction/model endpoint.
		// A viewer sees the current version number in the base view, never the
		// prompt or the model base URL.
		if manager && row.CurrentVersion > 0 {
			version, err := s.store.GetVersion(ctx, id, row.CurrentVersion)
			if err != nil {
				return err
			}
			published := versionView(version)
			view.Published = &published
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
		apiKeyEnc, _, err := sealAgentKey(input.APIKey)
		if err != nil {
			return err
		}
		mcpJSON, skillsJSON, err := s.validateExtensions(ctx, &input)
		if err != nil {
			return err
		}
		uid := extractUserIDFromContext(ctx)
		spec := domain.AgentSpec{Name: input.Name, Description: input.Description, Instruction: input.Instruction, Model: input.Model, Tools: string(tools), BaseURL: input.BaseURL, APIKeyEnc: apiKeyEnc, MaxTokens: input.MaxTokens, MCPBindings: mcpJSON, Skills: skillsJSON}
		row, err := s.store.CreateAgent(ctx, uid, spec, func(ctx context.Context, id int64) error { return s.auth.SetAgentOwner(ctx, id, uid) })
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
		apiKeyEnc, updateKey, err := sealAgentKey(input.APIKey)
		if err != nil {
			return err
		}
		mcpJSON, skillsJSON, err := s.validateExtensions(ctx, &input)
		if err != nil {
			return err
		}
		spec := domain.AgentSpec{Name: input.Name, Description: input.Description, Instruction: input.Instruction, Model: input.Model, Tools: string(tools), BaseURL: input.BaseURL, APIKeyEnc: apiKeyEnc, MaxTokens: input.MaxTokens, MCPBindings: mcpJSON, Skills: skillsJSON}
		row, err := s.store.UpdateDraft(ctx, id, input.Revision, spec, updateKey)
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
		// Bind the published tools in OpenFGA so tool execution can be
		// authorized per (agent, tool). Bindings accumulate across versions:
		// a tool removed in a newer version stays executable for runs that
		// were claimed under the older version snapshot.
		var tools []string
		if err := json.Unmarshal([]byte(version.Tools), &tools); err != nil {
			return err
		}
		var bindErr error
		var bindErrTool string
		for _, tool := range tools {
			if err := s.auth.SetAgentToolBinding(ctx, id, tool, true); err != nil {
				bindErrTool, bindErr = tool, err
				break
			}
		}
		var mcpBindings []domain.MCPBinding
		if version.MCPBindings != "" {
			if err := json.Unmarshal([]byte(version.MCPBindings), &mcpBindings); err != nil {
				return err
			}
		}
		for _, binding := range mcpBindings {
			if err := s.auth.SetAgentToolBinding(ctx, id, binding.Exposed, true); err != nil {
				if bindErr == nil {
					bindErrTool, bindErr = binding.Exposed, err
				}
				break
			}
		}
		if bindErr != nil {
			// Tool authorization is part of publishing: roll the version back
			// instead of leaving a published agent whose tools all fail
			// authorization. A concurrent newer publish aborts the rollback.
			if rerr := s.store.RollbackPublish(ctx, id, version.Version); rerr != nil {
				return fmt.Errorf("bind tool %q: %w (rollback failed: %v)", bindErrTool, bindErr, rerr)
			}
			return fmt.Errorf("bind tool %q: %w", bindErrTool, bindErr)
		}
		writeAgentJSON(w, http.StatusOK, versionView(version))
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
			return domain.ErrAgentNotFound
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
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.auth.CanManageAgent(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return authz.ErrDenied
		}
		limit := httpapi.Limit(r, 50, 200)
		offset, err := httpapi.Offset(r.URL.Query().Get("cursor"))
		if err != nil {
			return invalidAgentInput("invalid cursor")
		}
		runs, total, err := s.store.ListRunsPage(ctx, id, limit, offset)
		if err != nil {
			return err
		}
		hasMore := int64(offset+len(runs)) < total
		writeAgentJSON(w, http.StatusOK, httpapi.NewPage(runViews(runs), offset, limit, hasMore))
		return nil
	})
}

// ListApprovals returns the caller's approvals (default: pending), used by the
// approval inbox and multi-device status refresh.
func (s *AgentHTTP) ListApprovals(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		uid := extractUserIDFromContext(ctx)
		status := r.URL.Query().Get("status")
		switch status {
		case "", domain.ApprovalPending, domain.ApprovalApproved, domain.ApprovalRejected, domain.ApprovalExpired, domain.ApprovalConsumed:
		default:
			return invalidAgentInput("invalid approval status")
		}
		limit := httpapi.Limit(r, 50, 200)
		offset, err := httpapi.Offset(r.URL.Query().Get("cursor"))
		if err != nil {
			return invalidAgentInput("invalid cursor")
		}
		rows, total, err := s.store.ListApprovals(ctx, uid, status, limit, offset)
		if err != nil {
			return err
		}
		hasMore := int64(offset+len(rows)) < total
		writeAgentJSON(w, http.StatusOK, httpapi.NewPage(approvalViews(rows), offset, limit, hasMore))
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
			return err
		}
		uid := extractUserIDFromContext(ctx)
		if uid != run.ActorUserID && s.auth.CanManageAgent(ctx, uid, run.AgentID) != nil {
			return authz.ErrDenied
		}
		writeAgentJSON(w, http.StatusOK, runView(run))
		return nil
	})
}

func (s *AgentHTTP) RunAction(c khttp.Context, action string) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid run id")
		}
		run, err := s.store.GetRun(ctx, id)
		if err != nil {
			return err
		}
		uid := extractUserIDFromContext(ctx)
		if uid != run.ActorUserID && s.auth.CanManageAgent(ctx, uid, run.AgentID) != nil {
			return authz.ErrDenied
		}
		acknowledge := strings.EqualFold(r.URL.Query().Get("acknowledgeSideEffect"), "true")
		if err := s.store.SetRunAction(ctx, id, action, acknowledge); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"action": action})
		return nil
	})
}

// Delete removes an agent and its versions. Only the owner (or a manager)
// can delete; the bot user is preserved as an inert account.
func (s *AgentHTTP) Delete(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
		id, err := parseAgentID(c)
		if err != nil {
			return err
		}
		if err := s.auth.CanManageAgent(ctx, extractUserIDFromContext(ctx), id); err != nil {
			return authz.ErrDenied
		}
		tools, err := s.store.DeleteAgent(ctx, id)
		if err != nil {
			return err
		}
		// The agent row is already gone, so a stale tuple cannot authorize a
		// live agent; still reconcile OpenFGA and report the cleanup failure.
		if err := s.auth.DeleteAgentTuples(ctx, id, tools); err != nil {
			log.DefaultLogger.Log(log.LevelWarn, "msg", "delete agent FGA tuples", "agent_id", id, "error", err)
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return nil
	})
}

// ApprovalDetail returns one approval for status display on cards. Only the
// approver (or an agent manager) may read it.
func (s *AgentHTTP) ApprovalDetail(c khttp.Context) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid approval id")
		}
		approval, err := s.store.GetApproval(ctx, id)
		if err != nil {
			return err
		}
		run, err := s.store.GetRun(ctx, approval.RunID)
		if err != nil {
			return err
		}
		uid := extractUserIDFromContext(ctx)
		if uid != approval.ApproverID && s.auth.CanManageAgent(ctx, uid, run.AgentID) != nil {
			return authz.ErrDenied
		}
		writeAgentJSON(w, http.StatusOK, approvalView(approval))
		return nil
	})
}

// ApprovalAction resolves a pending agent tool approval. The approver is the
// run's actor; the transition is one-shot and the run is requeued for the
// agent worker to resume from its checkpoint.
func (s *AgentHTTP) ApprovalAction(c khttp.Context, action string) error {
	return s.handle(c, func(ctx context.Context, w http.ResponseWriter, _ *http.Request) error {
		id, err := strconv.ParseUint(c.Vars().Get("id"), 10, 64)
		if err != nil || id == 0 {
			return invalidAgentInput("invalid approval id")
		}
		var to string
		switch action {
		case "approve":
			to = "approved"
		case "reject":
			to = "rejected"
		default:
			return invalidAgentInput("invalid approval action")
		}
		approval, err := s.store.GetApproval(ctx, id)
		if err != nil {
			return err
		}
		if approval.Status != domain.ApprovalPending {
			return domain.ErrApprovalGone
		}
		run, err := s.store.GetRun(ctx, approval.RunID)
		if err != nil {
			return err
		}
		if run.Status != domain.RunWaitingApproval {
			return domain.ErrRunState
		}
		uid := extractUserIDFromContext(ctx)
		// The first-version approver is the initiating actor; the agent must
		// still be usable by them at decision time.
		if uid != approval.ApproverID || s.auth.CanUseAgent(ctx, uid, run.AgentID) != nil {
			return authz.ErrDenied
		}
		// Decision and requeue are one transaction: a crash can no longer leave
		// an approved approval attached to a permanently parked run.
		if err := s.store.DecideAndRequeue(ctx, id, to, uid); err != nil {
			return err
		}
		writeAgentJSON(w, http.StatusOK, map[string]string{"status": to})
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
	if err != nil || row.BotUserID != int64(botID) || row.CurrentVersion <= 0 || row.Status != "active" {
		return authz.ErrDenied
	}
	if err := s.auth.CanAccessTopic(ctx, int64(botID), rawTopic); err != nil {
		return authz.ErrDenied
	}
	return nil
}
