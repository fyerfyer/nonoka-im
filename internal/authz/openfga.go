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
	"time"

	fga "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
	"github.com/openfga/go-sdk/credentials"
	"nonoka-im/internal/topic"
)

//go:embed model.json
var modelFS embed.FS

var ErrDenied = errors.New("access denied")

type OpenFGA struct{ client *fgaclient.OpenFgaClient }

type AgentShare struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

func NewOpenFGA(apiURL, storeID, modelID string) (*OpenFGA, error) {
	if apiURL == "" || storeID == "" || modelID == "" {
		return nil, fmt.Errorf("OPENFGA_API_URL, OPENFGA_STORE_ID and OPENFGA_MODEL_ID are required")
	}
	c, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{ApiUrl: apiURL, StoreId: storeID, AuthorizationModelId: modelID, Credentials: openFGACredentials(), HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	if err != nil {
		return nil, err
	}
	return &OpenFGA{client: c}, nil
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
			if err == nil && latest != nil {
				latestModel := latest.GetAuthorizationModel()
				latestModelID = latestModel.GetId()
			}
			if latestModelID != "" {
				modelID = latestModelID
			} else {
				model, modelErr := Model()
				if modelErr != nil {
					return nil, modelErr
				}
				written, writeErr := configured.WriteAuthorizationModel(ctx).Body(*model).Execute()
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

func (a *OpenFGA) check(ctx context.Context, subject, relation, object string) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("OpenFGA unavailable")
	}
	result, err := a.client.Check(ctx).Body(fgaclient.ClientCheckRequest{User: subject, Relation: relation, Object: object}).Options(fgaclient.ClientCheckOptions{Consistency: fga.CONSISTENCYPREFERENCE_HIGHER_CONSISTENCY.Ptr()}).Execute()
	if err != nil {
		return fmt.Errorf("OpenFGA check: %w", err)
	}
	if !result.GetAllowed() {
		return ErrDenied
	}
	return nil
}

func (a *OpenFGA) CanAccessTopic(ctx context.Context, userID int64, raw string) error {
	if userID <= 0 {
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

func (a *OpenFGA) CanManageAgent(ctx context.Context, userID, agentID int64) error {
	return a.check(ctx, user(userID), "can_manage", agent(agentID))
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
	if err == nil || remove {
		return err
	}
	// Conflict options were added to OpenFGA in v1.10. Newer servers answer a
	// duplicate tuple write with HTTP 409; the bundled v1.8 stack instead
	// answers with HTTP 422 write_failed_due_to_invalid_input. Use only the
	// SDK's structured error fields (no message-text matching) to treat a
	// duplicate write as the idempotent success bootstrap and repeated sends
	// rely on.
	var apiErr fga.FgaApiError
	if errors.As(err, &apiErr) && apiErr.ResponseStatusCode() == http.StatusConflict {
		return nil
	}
	var validationErr fga.FgaApiValidationError
	if errors.As(err, &validationErr) && validationErr.ResponseCode() == fga.ERRORCODE_WRITE_FAILED_DUE_TO_INVALID_INPUT {
		// The 422 code also covers genuinely invalid writes, so confirm the
		// tuple is already present before calling it a duplicate (one extra
		// Read round trip on this rare path).
		if a.tupleExists(ctx, subject, relation, object) {
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
func (a *OpenFGA) SetAgentOwner(ctx context.Context, agentID, userID int64) error {
	return a.write(ctx, user(userID), "owner", agent(agentID), false)
}
func (a *OpenFGA) SetAgentShare(ctx context.Context, agentID int64, kind, target string, active bool) error {
	subject, relation := "", ""
	switch kind {
	case "user":
		subject, relation = "user:"+target, "user_grant"
	case "group":
		subject, relation = "group:"+target, "shared_group"
	default:
		return fmt.Errorf("unsupported sharing kind %q", kind)
	}
	return a.write(ctx, subject, relation, agent(agentID), !active)
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
			case "user_grant":
				shares = append(shares, AgentShare{Kind: "user", Target: strings.TrimPrefix(key.GetUser(), "user:")})
			case "shared_group":
				shares = append(shares, AgentShare{Kind: "group", Target: strings.TrimPrefix(key.GetUser(), "group:")})
			}
		}
		continuation = result.GetContinuationToken()
		if continuation == "" {
			return shares, nil
		}
	}
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
