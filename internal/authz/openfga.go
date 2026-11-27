package authz

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	fga "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
	"github.com/openfga/go-sdk/credentials"
	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/topic"
)

//go:embed model.json
var modelFS embed.FS

var ErrDenied = errors.New("access denied")

type OpenFGA struct {
	client *fgaclient.OpenFgaClient

	cacheMu  sync.RWMutex
	cache    map[string]cachedCheck
	cacheTTL time.Duration
}

type cachedCheck struct {
	allowed bool
	expires time.Time
}

// AgentShare is the domain type; the alias keeps existing authz callers
// source-compatible while management code depends on agent/domain only.
type AgentShare = domain.AgentShare

func NewOpenFGA(apiURL, storeID, modelID string) (*OpenFGA, error) {
	if apiURL == "" || storeID == "" || modelID == "" {
		return nil, fmt.Errorf("OPENFGA_API_URL, OPENFGA_STORE_ID and OPENFGA_MODEL_ID are required")
	}
	c, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{ApiUrl: apiURL, StoreId: storeID, AuthorizationModelId: modelID, Credentials: openFGACredentials(), HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	if err != nil {
		return nil, err
	}
	return &OpenFGA{client: c, cache: make(map[string]cachedCheck), cacheTTL: 30 * time.Second}, nil
}

// openFGACredentials returns preshared-key credentials when OPENFGA_API_TOKEN
// is set, or nil to keep the OpenFGA "none" authentication mode.
func openFGACredentials() *credentials.Credentials {
	token := strings.TrimSpace(os.Getenv("OPENFGA_API_TOKEN"))
	if token == "" {
		return nil
	}
	creds, err := credentials.NewCredentials(credentials.Credentials{
		Method: credentials.CredentialsMethodApiToken,
		Config: &credentials.Config{ApiToken: token},
	})
	if err != nil {
		return nil
	}
	return creds
}

func NewOpenFGAFromEnv() (*OpenFGA, error) {
	apiURL := os.Getenv("OPENFGA_API_URL")
	storeID := os.Getenv("OPENFGA_STORE_ID")
	modelID := os.Getenv("OPENFGA_MODEL_ID")
	if truthy(os.Getenv("OPENFGA_AUTO_BOOTSTRAP")) {
		return bootstrapOpenFGA(apiURL, storeID, modelID)
	}
	return NewOpenFGA(apiURL, storeID, modelID)
}

// bootstrapOpenFGA creates or reuses a development store and authorization
// model. Production deployments should pin OPENFGA_STORE_ID and
// OPENFGA_MODEL_ID instead; auto bootstrap exists so the bundled Docker stack
// can start without copying generated ULIDs into a checked-in config file.
func bootstrapOpenFGA(apiURL, storeID, modelID string) (*OpenFGA, error) {
	if strings.TrimSpace(apiURL) == "" {
		return nil, fmt.Errorf("OPENFGA_API_URL is required when auto bootstrap is enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	storeName := os.Getenv("OPENFGA_STORE_NAME")
	if storeName == "" {
		storeName = "nonoka-im"
	}

	var lastErr error
	for ctx.Err() == nil {
		client, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{ApiUrl: apiURL, Credentials: openFGACredentials()})
		if err != nil {
			return nil, err
		}
		if storeID == "" {
			name := storeName
			stores, err := client.ListStores(ctx).Options(fgaclient.ClientListStoresOptions{Name: &name}).Execute()
			if err != nil {
				lastErr = err
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if items := stores.GetStores(); len(items) > 0 {
				storeID = items[0].GetId()
			} else {
				created, err := client.CreateStore(ctx).Body(fgaclient.ClientCreateStoreRequest{Name: storeName}).Execute()
				if err != nil {
					lastErr = err
					time.Sleep(500 * time.Millisecond)
					continue
				}
				storeID = created.GetId()
			}
		}

		configured, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{ApiUrl: apiURL, StoreId: storeID, Credentials: openFGACredentials()})
		if err != nil {
			return nil, err
		}
		if modelID == "" {
			latest, err := configured.ReadLatestAuthorizationModel(ctx).Execute()
			latestModelID := ""
			var latestTypeDefs []fga.TypeDefinition
			if err == nil && latest != nil {
				latestModel := latest.GetAuthorizationModel()
				latestModelID = latestModel.GetId()
				latestTypeDefs = latestModel.GetTypeDefinitions()
			}
			desired, modelErr := Model()
			if modelErr != nil {
				return nil, modelErr
			}
			// Reuse the deployed model only when it matches the embedded one;
			// otherwise write the embedded model as a new version so added
			// types/relations (e.g. agent_tool) become available.
			sameModel := len(latestTypeDefs) == len(desired.TypeDefinitions)
			if sameModel {
				latestJSON, _ := json.Marshal(latestTypeDefs)
				desiredJSON, _ := json.Marshal(desired.TypeDefinitions)
				sameModel = string(latestJSON) == string(desiredJSON)
			}
			if latestModelID != "" && sameModel {
				modelID = latestModelID
			} else {
				written, writeErr := configured.WriteAuthorizationModel(ctx).Body(*desired).Execute()
				if writeErr != nil {
					lastErr = writeErr
					time.Sleep(500 * time.Millisecond)
					continue
				}
				modelID = written.GetAuthorizationModelId()
			}
		}
		return NewOpenFGA(apiURL, storeID, modelID)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("bootstrap OpenFGA: %w", lastErr)
	}
	return nil, fmt.Errorf("bootstrap OpenFGA: %w", ctx.Err())
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func user(id int64) string  { return "user:" + strconv.FormatInt(id, 10) }
func agent(id int64) string { return "agent:" + strconv.FormatInt(id, 10) }

// IsDenied reports whether err is an authorization policy denial rather than
// an OpenFGA outage. It satisfies agent/app.DeniedClassifier without making
// the application depend on this adapter package.
func (a *OpenFGA) IsDenied(err error) bool { return errors.Is(err, ErrDenied) }

func (a *OpenFGA) check(ctx context.Context, subject, relation, object string) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("OpenFGA unavailable")
	}
	key := subject + "|" + relation + "|" + object
	a.cacheMu.RLock()
	cached, ok := a.cache[key]
	a.cacheMu.RUnlock()
	if ok && time.Now().Before(cached.expires) {
		if cached.allowed {
			return nil
		}
		return ErrDenied
	}
	result, err := a.client.Check(ctx).Body(fgaclient.ClientCheckRequest{User: subject, Relation: relation, Object: object}).Options(fgaclient.ClientCheckOptions{Consistency: fga.CONSISTENCYPREFERENCE_MINIMIZE_LATENCY.Ptr()}).Execute()
	if err != nil {
		return fmt.Errorf("OpenFGA check: %w", err)
	}
	allowed := result.GetAllowed()
	ttl := a.cacheTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	a.cacheMu.Lock()
	if len(a.cache) > 100000 {
		now := time.Now()
		for k, entry := range a.cache {
			if now.After(entry.expires) {
				delete(a.cache, k)
			}
		}
	}
	a.cache[key] = cachedCheck{allowed: allowed, expires: time.Now().Add(ttl)}
	a.cacheMu.Unlock()
	if !allowed {
		return ErrDenied
	}
	return nil
}

// clearCache drops all cached checks. Writes are rare compared with checks;
// clearing everything keeps group membership and share changes immediately
// visible without maintaining reverse indexes.
func (a *OpenFGA) clearCache() {
	if a == nil {
		return
	}
	a.cacheMu.Lock()
	a.cache = make(map[string]cachedCheck)
	a.cacheMu.Unlock()
}

func (a *OpenFGA) CanAccessTopic(ctx context.Context, userID int64, raw string) error {
	if userID <= 0 {
		return ErrDenied
	}
	if allowed, handled := topic.AuthorizeDirect(userID, raw); handled {
		if allowed {
			return nil
		}
		return ErrDenied
	}
	normal, err := topic.NormalizeTopic(raw)
	if err != nil {
		return err
	}
	return a.check(ctx, user(userID), "can_access", "topic:"+normal)
}

func (a *OpenFGA) EnsureTopicForSend(ctx context.Context, userID int64, raw string) error {
	t, err := topic.NormalizeTopic(raw)
	if err != nil {
		return err
	}
	switch topic.ParseType(t) {
	case topic.TypeP2P:
		x, y, err := topic.ExtractUserIDsFromP2PTopic(t)
		if err != nil {
			return err
		}
		if userID != x && userID != y {
			return ErrDenied
		}
		if err := a.SetTopicParticipant(ctx, t, x); err != nil {
			return err
		}
		return a.SetTopicParticipant(ctx, t, y)
	case topic.TypeSystem:
		owner, err := strconv.ParseInt(t[len(topic.PrefixSystem):], 10, 64)
		if err != nil || owner != userID {
			return ErrDenied
		}
		return a.SetTopicParticipant(ctx, t, owner)
	default:
		return nil
	}
}

func (a *OpenFGA) CanReadFile(ctx context.Context, userID int64, fileID string) error {
	if userID <= 0 || fileID == "" {
		return ErrDenied
	}
	return a.check(ctx, user(userID), "can_read", "file:"+fileID)
}

// CanManagePlatform authorizes deployment-level administration (model
// services, platform review queues and policy). Platform admins are ordinary IM
// users; the role lives in OpenFGA so it can be granted/revoked without a
// second ACL.
func (a *OpenFGA) CanManagePlatform(ctx context.Context, userID int64) error {
	return a.check(ctx, user(userID), "can_manage", "platform:global")
}

// SetPlatformAdmin grants or revokes the platform administrator relation.
func (a *OpenFGA) SetPlatformAdmin(ctx context.Context, userID int64, enabled bool) error {
	return a.write(ctx, user(userID), "admin", "platform:global", !enabled)
}

func (a *OpenFGA) CanManageAgent(ctx context.Context, userID, agentID int64) error {
	return a.check(ctx, user(userID), "can_manage", agent(agentID))
}
func (a *OpenFGA) CanOwnAgent(ctx context.Context, userID, agentID int64) error {
	return a.check(ctx, user(userID), "owner", agent(agentID))
}
func (a *OpenFGA) CanViewAgent(ctx context.Context, userID, agentID int64) error {
	return a.check(ctx, user(userID), "can_view", agent(agentID))
}
func (a *OpenFGA) CanUseAgent(ctx context.Context, userID, agentID int64) error {
	return a.check(ctx, user(userID), "can_use", agent(agentID))
}
func (a *OpenFGA) CanManageGroup(ctx context.Context, userID int64, groupID string) error {
	return a.check(ctx, user(userID), "can_manage", "group:"+groupID)
}

func (a *OpenFGA) write(ctx context.Context, subject, relation, object string, remove bool) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("OpenFGA unavailable")
	}
	body := fgaclient.ClientWriteRequest{}
	if remove {
		body.Deletes = []fgaclient.ClientTupleKeyWithoutCondition{{User: subject, Relation: relation, Object: object}}
	} else {
		body.Writes = []fgaclient.ClientTupleKey{{User: subject, Relation: relation, Object: object}}
	}
	_, err := a.client.Write(ctx).Body(body).Options(fgaclient.ClientWriteOptions{Conflict: fgaclient.ClientWriteConflictOptions{OnDuplicateWrites: fgaclient.CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE, OnMissingDeletes: fgaclient.CLIENT_WRITE_REQUEST_ON_MISSING_DELETES_IGNORE}}).Execute()
	if err == nil {
		a.clearCache()
		return nil
	}
	// Conflict options were added to OpenFGA in v1.10. Newer servers answer a
	// duplicate tuple write or a missing delete as a no-op; the bundled v1.8
	// stack instead answers with HTTP 422 write_failed_due_to_invalid_input.
	// Use only the SDK's structured error fields (no message-text matching) and
	// confirm the tuple state before treating the failure as idempotent.
	var apiErr fga.FgaApiError
	if errors.As(err, &apiErr) && apiErr.ResponseStatusCode() == http.StatusConflict {
		a.clearCache()
		return nil
	}
	var validationErr fga.FgaApiValidationError
	if errors.As(err, &validationErr) && validationErr.ResponseCode() == fga.ERRORCODE_WRITE_FAILED_DUE_TO_INVALID_INPUT {
		exists := a.tupleExists(ctx, subject, relation, object)
		if remove {
			// Deleting a tuple that is already absent is a successful revoke.
			if !exists {
				a.clearCache()
				return nil
			}
			return err
		}
		// The 422 code also covers genuinely invalid writes, so confirm the
		// tuple is already present before calling it a duplicate (one extra
		// Read round trip on this rare path).
		if exists {
			a.clearCache()
			return nil
		}
	}
	return err
}

func (a *OpenFGA) tupleExists(ctx context.Context, subject, relation, object string) bool {
	user, rel, obj := subject, relation, object
	result, err := a.client.Read(ctx).Body(fgaclient.ClientReadRequest{User: &user, Relation: &rel, Object: &obj}).Options(fgaclient.ClientReadOptions{PageSize: fga.PtrInt32(1)}).Execute()
	if err != nil {
		return false
	}
	return len(result.GetTuples()) > 0
}

func (a *OpenFGA) SetGroupOwner(ctx context.Context, groupID string, userID int64) error {
	return a.write(ctx, user(userID), "owner", "group:"+groupID, false)
}
func (a *OpenFGA) SetGroupMember(ctx context.Context, groupID string, userID int64, active bool) error {
	return a.write(ctx, user(userID), "member", "group:"+groupID, !active)
}
func (a *OpenFGA) SetTopicGroup(ctx context.Context, raw, groupID string) error {
	t, err := topic.NormalizeTopic(raw)
	if err != nil {
		return err
	}
	return a.write(ctx, "group:"+groupID, "group", "topic:"+t, false)
}
func (a *OpenFGA) SetTopicParticipant(ctx context.Context, raw string, userID int64) error {
	t, err := topic.NormalizeTopic(raw)
	if err != nil {
		return err
	}
	return a.write(ctx, user(userID), "participant", "topic:"+t, false)
}
func (a *OpenFGA) SetFileOwner(ctx context.Context, fileID string, userID int64) error {
	return a.write(ctx, user(userID), "owner", "file:"+fileID, false)
}
func (a *OpenFGA) SetFileViewer(ctx context.Context, fileID string, userID int64, active bool) error {
	return a.write(ctx, user(userID), "viewer", "file:"+fileID, !active)
}
func (a *OpenFGA) SetFileSharedTopic(ctx context.Context, fileID, rawTopic string, active bool) error {
	t, err := topic.NormalizeTopic(rawTopic)
	if err != nil {
		return err
	}
	return a.write(ctx, "topic:"+t, "shared_topic", "file:"+fileID, !active)
}
func (a *OpenFGA) SetFileSharedGroup(ctx context.Context, fileID, groupID string, active bool) error {
	return a.write(ctx, "group:"+groupID, "shared_group", "file:"+fileID, !active)
}
func (a *OpenFGA) SetAgentToolBinding(ctx context.Context, agentID int64, tool string, active bool) error {
	return a.write(ctx, agent(agentID), "bound", "agent_tool:"+agentToolID(agentID, tool), !active)
}

// CanExecuteTool authorizes one tool call for a run. It combines, inside the
// permission module: the actor may use the agent, the published agent version
// binds the tool, and the actor may access the target resource within the
// current topic scope. Any OpenFGA failure denies.
//
// resourceType is "file" (resourceRef is a file ID, checked with CanReadFile)
// or "topic" (resourceRef must be empty or equal currentTopic, then checked
// with CanAccessTopic, keeping topic-scoped tools on the current conversation).
func (a *OpenFGA) CanExecuteTool(ctx context.Context, actorID, agentID int64, tool, currentTopic, resourceType, resourceRef string) error {
	if actorID <= 0 || agentID <= 0 || tool == "" {
		return ErrDenied
	}
	if err := a.check(ctx, user(actorID), "can_use", agent(agentID)); err != nil {
		return err
	}
	if err := a.check(ctx, agent(agentID), "can_execute", "agent_tool:"+agentToolID(agentID, tool)); err != nil {
		return err
	}
	switch resourceType {
	case "file":
		if resourceRef == "" {
			return ErrDenied
		}
		return a.CanReadFile(ctx, actorID, resourceRef)
	case "topic":
		if resourceRef != "" && resourceRef != currentTopic {
			return ErrDenied
		}
		return a.CanAccessTopic(ctx, actorID, currentTopic)
	case "external":
		// External (MCP) tools carry no IM resource; binding + agent use
		// authorization above is the full scope.
		return nil
	default:
		return ErrDenied
	}
}

func agentToolID(agentID int64, tool string) string {
	// OpenFGA object IDs cannot contain ':'; '.' separates agent and tool.
	return strconv.FormatInt(agentID, 10) + "." + tool
}
func (a *OpenFGA) SetAgentOwner(ctx context.Context, agentID, userID int64) error {
	return a.write(ctx, user(userID), "owner", agent(agentID), false)
}
func (a *OpenFGA) SetAgentShare(ctx context.Context, agentID int64, kind, target, role string, active bool) error {
	if role == "" {
		role = "viewer"
	}
	subject := ""
	relations := []string{}
	switch kind {
	case "user":
		subject = "user:" + target
		switch role {
		case "viewer":
			// user_grant is the legacy viewer relation; remove it together
			// with viewer so pre-role tuples do not linger.
			relations = []string{"viewer", "user_grant"}
		case "editor":
			relations = []string{"editor"}
		default:
			return fmt.Errorf("unsupported user role %q", role)
		}
	case "group":
		subject = "group:" + target
		switch role {
		case "viewer":
			relations = []string{"shared_group"}
		case "editor":
			relations = []string{"shared_group_editor"}
		default:
			return fmt.Errorf("unsupported group role %q", role)
		}
	default:
		return fmt.Errorf("unsupported sharing kind %q", kind)
	}
	allRelations := relations
	if kind == "user" {
		allRelations = []string{"viewer", "user_grant", "editor"}
	} else {
		allRelations = []string{"shared_group", "shared_group_editor"}
	}
	if active {
		for _, relation := range allRelations {
			if relation == relations[0] {
				continue
			}
			_ = a.write(ctx, subject, relation, agent(agentID), true)
		}
		return a.write(ctx, subject, relations[0], agent(agentID), false)
	}
	var firstErr error
	for _, relation := range allRelations {
		if err := a.write(ctx, subject, relation, agent(agentID), true); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (a *OpenFGA) ListAgentShares(ctx context.Context, agentID int64) ([]AgentShare, error) {
	object := agent(agentID)
	shares := make([]AgentShare, 0)
	var continuation string
	for {
		options := fgaclient.ClientReadOptions{Consistency: fga.CONSISTENCYPREFERENCE_HIGHER_CONSISTENCY.Ptr()}
		if continuation != "" {
			options.ContinuationToken = &continuation
		}
		result, err := a.client.Read(ctx).Body(fgaclient.ClientReadRequest{Object: &object}).Options(options).Execute()
		if err != nil {
			return nil, err
		}
		for _, tuple := range result.GetTuples() {
			key := tuple.GetKey()
			switch key.GetRelation() {
			case "user_grant", "viewer":
				shares = append(shares, AgentShare{Kind: "user", Target: strings.TrimPrefix(key.GetUser(), "user:"), Role: "viewer"})
			case "editor":
				shares = append(shares, AgentShare{Kind: "user", Target: strings.TrimPrefix(key.GetUser(), "user:"), Role: "editor"})
			case "shared_group":
				shares = append(shares, AgentShare{Kind: "group", Target: strings.TrimPrefix(key.GetUser(), "group:"), Role: "viewer"})
			case "shared_group_editor":
				shares = append(shares, AgentShare{Kind: "group", Target: strings.TrimPrefix(key.GetUser(), "group:"), Role: "editor"})
			}
		}
		continuation = result.GetContinuationToken()
		if continuation == "" {
			return shares, nil
		}
	}
}

// DeleteAgentTuples removes every authorization tuple that references the
// agent: shares/owner (object=agent:<id>) and tool bindings
// (user=agent:<id>). It is called after the agent row is deleted; a failed
// cleanup leaves only inert tuples because the agent can no longer be loaded.
func (a *OpenFGA) DeleteAgentTuples(ctx context.Context, agentID int64, tools []string) error {
	id := agent(agentID)
	seen := make(map[string]fgaclient.ClientTupleKeyWithoutCondition)
	read := func(filter fgaclient.ClientReadRequest) ([]fgaclient.ClientTupleKeyWithoutCondition, error) {
		var out []fgaclient.ClientTupleKeyWithoutCondition
		var continuation string
		for {
			options := fgaclient.ClientReadOptions{PageSize: fga.PtrInt32(100)}
			if continuation != "" {
				options.ContinuationToken = &continuation
			}
			result, err := a.client.Read(ctx).Body(filter).Options(options).Execute()
			if err != nil {
				return nil, err
			}
			for _, tuple := range result.GetTuples() {
				key := tuple.GetKey()
				out = append(out, fgaclient.ClientTupleKeyWithoutCondition{
					User: key.GetUser(), Relation: key.GetRelation(), Object: key.GetObject(),
				})
			}
			continuation = result.GetContinuationToken()
			if continuation == "" {
				return out, nil
			}
		}
	}
	// Tuples where the agent is the object (owner, viewer, grants, shares).
	objectTuples, err := read(fgaclient.ClientReadRequest{Object: &id})
	if err != nil {
		return err
	}
	for _, t := range objectTuples {
		seen[t.User+"|"+t.Relation+"|"+t.Object] = t
	}
	// Tool bindings are keyed by agent_tool:<agentID>.<tool>; construct them
	// directly instead of listing by subject (the Read API requires an object
	// type when the subject filter is used).
	for _, tool := range tools {
		t := fgaclient.ClientTupleKeyWithoutCondition{
			User: id, Relation: "bound", Object: "agent_tool:" + agentToolID(agentID, tool),
		}
		seen[t.User+"|"+t.Relation+"|"+t.Object] = t
	}
	if len(seen) == 0 {
		return nil
	}
	deletes := make([]fgaclient.ClientTupleKeyWithoutCondition, 0, len(seen))
	for _, t := range seen {
		deletes = append(deletes, t)
	}
	const batch = 50
	for start := 0; start < len(deletes); start += batch {
		end := start + batch
		if end > len(deletes) {
			end = len(deletes)
		}
		body := fgaclient.ClientWriteRequest{Deletes: deletes[start:end]}
		if _, err := a.client.Write(ctx).Body(body).Options(fgaclient.ClientWriteOptions{
			Conflict: fgaclient.ClientWriteConflictOptions{
				OnDuplicateWrites: fgaclient.CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE,
				OnMissingDeletes:  fgaclient.CLIENT_WRITE_REQUEST_ON_MISSING_DELETES_IGNORE,
			},
		}).Execute(); err != nil {
			return err
		}
	}
	return nil
}

func Model() (*fga.WriteAuthorizationModelRequest, error) {
	b, err := modelFS.ReadFile("model.json")
	if err != nil {
		return nil, err
	}
	var model fga.WriteAuthorizationModelRequest
	if err := json.Unmarshal(b, &model); err != nil {
		return nil, err
	}
	return &model, nil
}
