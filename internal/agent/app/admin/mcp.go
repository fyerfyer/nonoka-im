package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nonoka-im/internal/agent/domain"

	"github.com/go-kratos/kratos/v2/log"
)

var mcpNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// MCPInput is the transport-neutral MCP server registration/edit request.
type MCPInput struct {
	Name        string
	DisplayName string
	Description string
	URL         string
	Transport   string
	AuthType    string
	HeaderName  string
	Credential  string
	Visibility  string
	Enabled     bool
}

// MCPConnectionInput is one user's credential for a server.
type MCPConnectionInput struct {
	AuthType   string
	HeaderName string
	Credential string
}

// MCPService owns the self-service MCP registry, discovery, connections and
// review. Server owners manage their own entries; platform admins can review
// and operate every entry.
type MCPService struct {
	Store     PlatformStore
	Authz     Authorizer
	Discovery MCPDiscovery
	Sealer    CredentialSealer
	Log       *log.Helper
}

func NewMCPService(store PlatformStore, az Authorizer, discovery MCPDiscovery, sealer CredentialSealer, logger log.Logger) *MCPService {
	return &MCPService{Store: store, Authz: az, Discovery: discovery, Sealer: sealer, Log: log.NewHelper(logger)}
}

func (s *MCPService) isAdmin(ctx context.Context, userID int64) bool {
	return s.Authz != nil && s.Authz.CanManagePlatform(ctx, userID) == nil
}

func (s *MCPService) List(ctx context.Context, userID int64) ([]domain.MCPServer, error) {
	if userID <= 0 {
		return nil, ErrForbidden
	}
	return s.Store.ListServersForUser(ctx, userID, s.isAdmin(ctx, userID))
}

func (s *MCPService) Get(ctx context.Context, userID int64, id uint64) (*domain.MCPServer, error) {
	server, err := s.Store.GetServerByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.canView(ctx, userID, server) {
		return nil, ErrForbidden
	}
	return server, nil
}

func (s *MCPService) Register(ctx context.Context, userID int64, input MCPInput) (*domain.MCPServer, error) {
	if userID <= 0 {
		return nil, ErrForbidden
	}
	if err := normalizeMCPInput(&input); err != nil {
		return nil, err
	}
	plain, sealed, err := s.sealInputCredential(input.AuthType, input.HeaderName, input.Credential)
	if err != nil {
		return nil, err
	}
	target := domain.MCPTarget{URL: input.URL, Transport: input.Transport, AuthType: input.AuthType, Credential: plain}
	discovered, err := s.Discovery.Discover(ctx, target)
	if err != nil {
		return nil, err
	}
	if discovered.NeedsAuth {
		return nil, domain.ErrMCPAuthRequired
	}
	review := domain.ReviewApproved
	if input.Visibility != domain.VisibilityPrivate {
		review = domain.ReviewPending
	}
	server, err := s.Store.CreateServer(ctx, domain.MCPServer{
		OwnerUserID: userID, Name: input.Name, DisplayName: input.DisplayName,
		Description: input.Description, Transport: input.Transport, URL: input.URL,
		AuthType: input.AuthType, CredentialEnc: sealed, Visibility: input.Visibility,
		ReviewStatus: review, Enabled: true,
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.ReplaceTools(ctx, server.ID, discovered.Tools); err != nil {
		_ = s.Store.DeleteServer(ctx, server.ID)
		return nil, err
	}
	_ = s.Store.SetServerHealth(ctx, server.ID, domain.MCPHealthHealthy, "")
	return s.Store.GetServerByID(ctx, server.ID)
}

func (s *MCPService) Update(ctx context.Context, userID int64, id uint64, input MCPInput) (*domain.MCPServer, error) {
	server, err := s.Store.GetServerByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.canManage(ctx, userID, server) {
		return nil, ErrForbidden
	}
	if err := normalizeMCPInput(&input); err != nil {
		return nil, err
	}
	changedConnection := input.URL != server.URL || input.AuthType != server.AuthType || input.Credential != ""
	plain := input.Credential
	if input.Credential == "" && input.AuthType == server.AuthType {
		if server.CredentialEnc != "" {
			plain, err = s.Sealer.Open(server.CredentialEnc)
			if err != nil {
				return nil, err
			}
		}
	}
	sealed := ""
	if input.Credential != "" {
		_, sealed, err = s.sealInputCredential(input.AuthType, input.HeaderName, input.Credential)
		if err != nil {
			return nil, err
		}
	}
	if input.Visibility != server.Visibility && input.Visibility != domain.VisibilityPrivate {
		server.ReviewStatus = domain.ReviewPending
	}
	if input.Visibility == domain.VisibilityPrivate {
		server.ReviewStatus = domain.ReviewApproved
	}
	server.Name = input.Name
	server.DisplayName = input.DisplayName
	server.Description = input.Description
	server.Transport = input.Transport
	server.URL = input.URL
	server.AuthType = input.AuthType
	server.Visibility = input.Visibility
	server.Enabled = input.Enabled
	if sealed != "" {
		server.CredentialEnc = sealed
	}
	var discovered *domain.MCPDiscoveryResult
	if changedConnection {
		target := mcpTarget(server, plain)
		discovered, err = s.Discovery.Discover(ctx, target)
		if err != nil {
			return nil, err
		}
		if discovered.NeedsAuth {
			return nil, domain.ErrMCPAuthRequired
		}
	}
	if err := s.Store.UpdateServer(ctx, *server); err != nil {
		return nil, err
	}
	if discovered != nil {
		if _, err := s.Store.ReplaceTools(ctx, server.ID, discovered.Tools); err != nil {
			return nil, err
		}
		_ = s.Store.SetServerHealth(ctx, server.ID, domain.MCPHealthHealthy, "")
	}
	return s.Store.GetServerByID(ctx, id)
}

func (s *MCPService) SetStatus(ctx context.Context, userID int64, id uint64, enabled bool) error {
	server, err := s.Store.GetServerByID(ctx, id)
	if err != nil {
		return err
	}
	if !s.canManage(ctx, userID, server) {
		return ErrForbidden
	}
	return s.Store.SetServerEnabled(ctx, id, enabled)
}

func (s *MCPService) Delete(ctx context.Context, userID int64, id uint64) error {
	server, err := s.Store.GetServerByID(ctx, id)
	if err != nil {
		return err
	}
	if !s.canManage(ctx, userID, server) {
		return ErrForbidden
	}
	return s.Store.DeleteServer(ctx, id)
}

func (s *MCPService) Discover(ctx context.Context, userID int64, id uint64) ([]domain.MCPTool, error) {
	server, err := s.Store.GetServerByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.canManage(ctx, userID, server) {
		return nil, ErrForbidden
	}
	plain, err := s.serverCredential(server)
	if err != nil {
		return nil, err
	}
	if err := s.discoverAndStore(ctx, server, plain); err != nil {
		return nil, err
	}
	return s.Store.ListTools(ctx, id)
}

func (s *MCPService) Test(ctx context.Context, userID int64, id uint64) error {
	server, err := s.Store.GetServerByID(ctx, id)
	if err != nil {
		return err
	}
	if !s.canView(ctx, userID, server) {
		return ErrForbidden
	}
	plain, err := s.serverCredential(server)
	if err != nil {
		return err
	}
	target := mcpTarget(server, plain)
	if err := s.Discovery.Test(ctx, target); err != nil {
		_ = s.Store.SetServerHealth(ctx, id, healthForError(err), err.Error())
		return err
	}
	return s.Store.SetServerHealth(ctx, id, domain.MCPHealthHealthy, "")
}

func (s *MCPService) Review(ctx context.Context, userID int64, id uint64, status, reason string) error {
	if !s.isAdmin(ctx, userID) {
		return ErrAdminRequired
	}
	if status != domain.ReviewApproved && status != domain.ReviewRejected {
		return fmt.Errorf("%w: review status must be approved or rejected", ErrInvalidInput)
	}
	return s.Store.SetServerReview(ctx, id, status, reason, userID)
}

func (s *MCPService) ListTools(ctx context.Context, userID int64, id uint64) ([]domain.MCPTool, error) {
	server, err := s.Store.GetServerByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.canView(ctx, userID, server) {
		return nil, ErrForbidden
	}
	return s.Store.ListTools(ctx, id)
}

func (s *MCPService) Connect(ctx context.Context, userID int64, serverID uint64, input MCPConnectionInput) (*domain.MCPConnection, error) {
	if userID <= 0 {
		return nil, ErrForbidden
	}
	server, err := s.Store.GetServerByID(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if !s.canUse(ctx, userID, server) {
		return nil, ErrForbidden
	}
	if input.AuthType == "" {
		input.AuthType = server.AuthType
	}
	plain, sealed, err := s.sealInputCredential(input.AuthType, input.HeaderName, input.Credential)
	if err != nil {
		return nil, err
	}
	target := mcpTarget(server, plain)
	target.AuthType = input.AuthType
	if err := s.Discovery.Test(ctx, target); err != nil {
		return nil, fmt.Errorf("mcp connection test failed: %w", err)
	}
	return s.Store.UpsertConnection(ctx, domain.MCPConnection{
		ServerID: serverID, UserID: userID, AuthType: input.AuthType,
		CredentialEnc: sealed, Status: "connected",
	})
}

func (s *MCPService) Disconnect(ctx context.Context, userID int64, serverID uint64) error {
	return s.Store.DeleteConnection(ctx, serverID, userID)
}

func (s *MCPService) ListConnections(ctx context.Context, userID int64) ([]domain.MCPConnection, error) {
	if userID <= 0 {
		return nil, ErrForbidden
	}
	return s.Store.ListConnections(ctx, userID)
}

// HealthCheckAll probes every enabled server with its server-level credential.
// Auth-required servers without an owner credential are marked needs_auth.
func (s *MCPService) HealthCheckAll(ctx context.Context) {
	servers, err := s.Store.ListAllServers(ctx, true)
	if err != nil {
		s.Log.Warnf("mcp health: list servers: %v", err)
		return
	}
	for _, server := range servers {
		plain, err := s.serverCredential(&server)
		if err != nil {
			_ = s.Store.SetServerHealth(ctx, server.ID, healthForError(err), err.Error())
			continue
		}
		target := mcpTarget(&server, plain)
		if err := s.Discovery.Test(ctx, target); err != nil {
			_ = s.Store.SetServerHealth(ctx, server.ID, healthForError(err), err.Error())
			continue
		}
		_ = s.Store.SetServerHealth(ctx, server.ID, domain.MCPHealthHealthy, "")
	}
}

func (s *MCPService) HealthCheckLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.HealthCheckAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.HealthCheckAll(ctx)
		}
	}
}

func (s *MCPService) canView(ctx context.Context, userID int64, server *domain.MCPServer) bool {
	if s.isAdmin(ctx, userID) || server.OwnerUserID == userID {
		return true
	}
	return server.Visibility != domain.VisibilityPrivate && server.ReviewStatus == domain.ReviewApproved
}

func (s *MCPService) canUse(ctx context.Context, userID int64, server *domain.MCPServer) bool {
	if !s.canView(ctx, userID, server) {
		return false
	}
	return server.Enabled
}

func (s *MCPService) canManage(ctx context.Context, userID int64, server *domain.MCPServer) bool {
	return s.isAdmin(ctx, userID) || server.OwnerUserID == userID
}

func (s *MCPService) discoverAndStore(ctx context.Context, server *domain.MCPServer, plain string) error {
	target := mcpTarget(server, plain)
	res, err := s.Discovery.Discover(ctx, target)
	if err != nil {
		_ = s.Store.SetServerHealth(ctx, server.ID, healthForError(err), err.Error())
		return err
	}
	if res.NeedsAuth {
		_ = s.Store.SetServerHealth(ctx, server.ID, domain.MCPHealthNeedsAuth, "authentication required")
		return domain.ErrMCPAuthRequired
	}
	if _, err := s.Store.ReplaceTools(ctx, server.ID, res.Tools); err != nil {
		return err
	}
	return s.Store.SetServerHealth(ctx, server.ID, domain.MCPHealthHealthy, "")
}

func (s *MCPService) serverCredential(server *domain.MCPServer) (string, error) {
	if server.AuthType == "" || server.AuthType == domain.MCPAuthNone || server.CredentialEnc == "" {
		return "", nil
	}
	if s.Sealer == nil {
		return "", fmt.Errorf("MCP credential sealer unavailable")
	}
	return s.Sealer.Open(server.CredentialEnc)
}

func (s *MCPService) sealInputCredential(authType, headerName, credential string) (plain, sealed string, err error) {
	if authType == "" {
		authType = domain.MCPAuthNone
	}
	switch authType {
	case domain.MCPAuthNone:
		return "", "", nil
	case domain.MCPAuthBearer:
		if strings.TrimSpace(credential) == "" {
			return "", "", fmt.Errorf("%w: bearer token is required", ErrInvalidInput)
		}
	case domain.MCPAuthHeader:
		if strings.TrimSpace(credential) == "" || strings.TrimSpace(headerName) == "" {
			return "", "", fmt.Errorf("%w: header name and value are required", ErrInvalidInput)
		}
		raw, _ := json.Marshal(map[string]string{"name": headerName, "value": credential})
		plain = string(raw)
	default:
		return "", "", fmt.Errorf("%w: auth type must be none, bearer or header", ErrInvalidInput)
	}
	if plain == "" {
		plain = credential
	}
	if s.Sealer == nil {
		return "", "", fmt.Errorf("MCP credential sealer unavailable")
	}
	sealed, err = s.Sealer.Seal(plain)
	if err != nil {
		return "", "", err
	}
	return plain, sealed, nil
}

func normalizeMCPInput(input *MCPInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.URL = strings.TrimSpace(input.URL)
	input.Visibility = strings.TrimSpace(input.Visibility)
	input.AuthType = strings.TrimSpace(input.AuthType)
	input.Transport = strings.TrimSpace(input.Transport)
	if input.Name == "" || !mcpNamePattern.MatchString(input.Name) {
		return fmt.Errorf("%w: name must match %s", ErrInvalidInput, mcpNamePattern.String())
	}
	if input.DisplayName == "" {
		input.DisplayName = input.Name
	}
	if len(input.DisplayName) > 200 || len(input.Description) > 4000 {
		return fmt.Errorf("%w: display name or description is too long", ErrInvalidInput)
	}
	if input.Transport == "" {
		input.Transport = domain.MCPTransportHTTP
	}
	if input.Transport != domain.MCPTransportHTTP {
		return fmt.Errorf("%w: only streamable_http is supported", ErrInvalidInput)
	}
	if input.AuthType == "" {
		input.AuthType = domain.MCPAuthNone
	}
	if input.AuthType != domain.MCPAuthNone && input.AuthType != domain.MCPAuthBearer && input.AuthType != domain.MCPAuthHeader {
		return fmt.Errorf("%w: invalid auth type", ErrInvalidInput)
	}
	if input.Visibility == "" {
		input.Visibility = domain.VisibilityPrivate
	}
	if input.Visibility != domain.VisibilityPrivate && input.Visibility != domain.VisibilityShared && input.Visibility != domain.VisibilityPublic {
		return fmt.Errorf("%w: invalid visibility", ErrInvalidInput)
	}
	return nil
}

func mcpTarget(server *domain.MCPServer, plain string) domain.MCPTarget {
	return domain.MCPTarget{URL: server.URL, Transport: server.Transport, AuthType: server.AuthType, Credential: plain}
}

func healthForError(err error) string {
	if err == nil {
		return domain.MCPHealthHealthy
	}
	if errors.Is(err, domain.ErrMCPAuthRequired) {
		return domain.MCPHealthNeedsAuth
	}
	return domain.MCPHealthError
}
