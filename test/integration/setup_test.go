package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/agent/app"
	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/agent/domain"
	agentmcp "nonoka-im/internal/agent/mcp"
	"nonoka-im/internal/audit"
	"nonoka-im/internal/authz"
	"nonoka-im/internal/biz"
	botadapters "nonoka-im/internal/bot/adapters"
	botapp "nonoka-im/internal/bot/app"
	botdelivery "nonoka-im/internal/bot/delivery"
	botstream "nonoka-im/internal/bot/stream"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	agentdata "nonoka-im/internal/data/agent"
	botdata "nonoka-im/internal/data/bot"
	imdata "nonoka-im/internal/data/im"
	platformdata "nonoka-im/internal/data/platform"
	"nonoka-im/internal/gateway"
	imapp "nonoka-im/internal/im/app"
	imconsumer "nonoka-im/internal/im/consumer"
	imgroup "nonoka-im/internal/im/group"
	impush "nonoka-im/internal/im/push"
	"nonoka-im/internal/im/search"
	"nonoka-im/internal/im/seq"
	mongostore "nonoka-im/internal/im/storage/mongo"
	kafkax "nonoka-im/internal/kafka"
	"nonoka-im/internal/llm"
	messagingim "nonoka-im/internal/messaging/im"
	"nonoka-im/internal/metrics"
	"nonoka-im/internal/realtime"
	"nonoka-im/internal/server"
	"nonoka-im/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
)

const (
	testHTTPAddr = "0.0.0.0:18000"
	testBaseURL  = "http://127.0.0.1:18000"
	testWSURL    = "ws://127.0.0.1:18000/ws"

	testKafkaBroker = "127.0.0.1:9092"
	testKafkaTopic  = "im-messages-test"

	testOpenFGAURL          = "http://127.0.0.1:8081"
	testOpenFGADefaultToken = "nonoka-openfga-test-key"
	testMeilisearchURL      = "http://127.0.0.1:7700"
	testMeilisearchKey      = "nonoka-search-test-key"
	testMeilisearchIndex    = "messages-test"
)

var (
	testLogger = log.NewStdLogger(os.Stdout)
	httpClient = &http.Client{Timeout: 5 * time.Second}

	// Seeded manual ModelService/Model for management API tests. The tests in
	// this package run sequentially; a fresh server re-seeds these IDs.
	testModelServiceID int64
	testModelID        int64
)

type testServer struct {
	data           *data.Data
	cleanup        func()
	httpSrv        *khttp.Server
	gwManager      *gateway.Manager
	gwSessionMgr   *gateway.SessionManager
	redis          redis.UniversalClient
	authConf       *conf.Auth
	kafkaProducer  gateway.MessageProducer
	kafkaTopic     string
	mongoDB        *mongo.Database
	storage        *mongostore.Store
	searchIndex    *search.Client
	msgWorker      *imconsumer.Worker
	messageApp     *imapp.MessageService
	query          *imapp.QueryService
	groupRepo      imdata.GroupRepo
	botIndex       *imgroup.BotIndex
	fileSvc        *service.FileService
	grpcCleanup    func()
	openFGA        *authz.OpenFGA
	fgaCleanup     func()
	modelServiceID int64
	modelID        int64
	modelSvc       *llm.Service
	botStore       *botdata.Store
	botDelivery    *botapp.DeliveryService
	botStreams     *botapp.StreamService
}

type testUserDirectory struct{ repo biz.AuthRepo }

func (u testUserDirectory) UserExists(ctx context.Context, userID int64) (bool, error) {
	_, err := u.repo.GetUserByID(ctx, userID)
	return err == nil, nil
}

func setupTestServer(t *testing.T, useKafka bool) *testServer {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires postgres, redis, mongodb and kafka")
	}
	ctx := context.Background()
	confData := &conf.Data{
		Database: &conf.Data_Database{Source: "host=127.0.0.1 user=postgres password=root dbname=nonoka_im_test port=5433 sslmode=disable TimeZone=Asia/Shanghai"},
		Redis:    &conf.Data_Redis{Addr: "127.0.0.1:6380"},
	}
	d, cleanup, err := data.NewData(confData)
	if err != nil {
		t.Fatalf("failed to create data layer: %v", err)
	}
	if err := d.CleanTestData(); err != nil {
		t.Fatalf("failed to clean test data: %v", err)
	}
	if d.Redis != nil {
		if err := d.Redis.FlushDB(ctx).Err(); err != nil {
			t.Fatalf("failed to flush redis test db: %v", err)
		}
	}

	mongoClient, err := mongo.Connect(options.Client().ApplyURI(testMongoURI))
	if err != nil {
		t.Fatalf("failed to connect to mongodb: %v", err)
	}
	mongoDB := mongoClient.Database(testMongoDB)
	for _, coll := range []string{"messages", "inboxes", "topic_seqs", "mention_inboxes", "agent_event_outbox", "search_index_outbox", "im_files"} {
		_ = mongoDB.Collection(coll).Drop(ctx)
	}
	storage := mongostore.NewStore(mongoDB, testLogger)
	searchIndex := search.NewClient(testMeilisearchURL, testMeilisearchKey, testMeilisearchIndex, 5*time.Second)
	if err := searchIndex.EnsureIndex(ctx); err != nil {
		t.Fatalf("failed to initialize search index: %v", err)
	}
	if err := searchIndex.ClearIndex(ctx); err != nil {
		t.Fatalf("failed to clear search index: %v", err)
	}
	storage.SetSearchEnabled(true)
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("failed to ensure mongodb indexes: %v", err)
	}

	authConf := &conf.Auth{
		JwtSecret:  "test-jwt-secret-do-not-use-in-production",
		TokenTtl:   durationpb.New(time.Hour),
		BcryptCost: int32(bcrypt.MinCost),
	}
	authRepo := imdata.NewAuthRepo(d.DB(), d.Redis, testLogger)
	authUC := biz.NewAuthUsecase(authRepo, authConf)
	authSvc := service.NewAuthService(authUC)
	dispatchSvc := service.NewDispatchService(d.Redis, &conf.Dispatch{Strategy: "consistent_hash"}, testLogger)
	fgaAuthorizer, fgaCleanup := setupOpenFGA(t)

	gwManager := gateway.NewManager(testLogger)
	gwSessionMgr := gateway.NewSessionManager(d.Redis, "test-gateway-node")
	var msgProducer gateway.MessageProducer
	var kafkaTopic string
	if useKafka {
		if err := waitForKafka(testKafkaBroker); err != nil {
			t.Fatalf("kafka not ready: %v", err)
		}
		kafkaTopic = testKafkaTopic
		if err := recreateTopicStable(testKafkaBroker, kafkaTopic, 3); err != nil {
			t.Fatalf("failed to create kafka topic: %v", err)
		}
		msgProducer = gateway.NewKafkaProducer(gateway.KafkaConfig{
			Brokers: []string{testKafkaBroker}, Topic: kafkaTopic, BatchSize: 10, MaxAttempts: 30,
		}, testLogger)
	} else {
		msgProducer = gateway.NewNoopProducer()
	}

	testMetrics := metrics.NewMetrics()
	conversationRepo := imdata.NewConversationRepo(d.DB(), testLogger)
	groupRepo := imdata.NewGroupRepo(d.DB(), testLogger)
	groupMemberRepo := imdata.NewGroupMemberRepo(d.DB(), testLogger)
	groupMembers := imgroup.NewService(groupMemberRepo, d.Redis, testLogger)
	botIndex := imgroup.NewBotIndexWithQuery(d.ListBotUserIDs, d.IsBotUser, testLogger)
	if err := botIndex.Refresh(ctx); err != nil {
		t.Fatalf("refresh bot index: %v", err)
	}

	fileStore := mongostore.NewFileStore(mongoDB)
	fileApp := &imapp.FileService{Store: fileStore, Authorizer: fgaAuthorizer, MaxUpload: service.DefaultMaxUploadBytes}
	messageApp := &imapp.MessageService{
		Writer: storage, IDs: seq.NewIDs(1), Seqs: seq.New(d.Redis, testLogger),
		Conversations: conversationRepo, GroupMembers: groupMembers, Bots: botIndex,
		Authorizer: fgaAuthorizer, Producer: gateway.NewDomainProducer(msgProducer),
		Attachments: fileApp, Metrics: testMetrics, Log: log.NewHelper(testLogger),
	}
	query := &imapp.QueryService{Reader: storage, Search: searchIndex, Authorizer: fgaAuthorizer, Log: log.NewHelper(testLogger)}

	gwHandler := gateway.NewHandler(gwManager, gwSessionMgr, messageApp, query, []byte(authConf.JwtSecret), gateway.HeartbeatConfig{
		Interval: 30 * time.Second, Timeout: 90 * time.Second,
	}, testLogger)
	gwHandler.SetMessageRateLimit(0, 0)
	wsServer := gateway.NewWebSocketServer(gwHandler, testLogger, 60*time.Second, 10*time.Second, nil)

	msgSvc := service.NewMessageService(messageApp, query, testLogger)
	userSvc := service.NewUserService(authUC)
	conversationSvc := service.NewConversationService(&imapp.ConversationService{Repo: conversationRepo})
	groupSvc := service.NewGroupService(&imapp.GroupService{Repo: groupRepo, Authorizer: fgaAuthorizer})
	agentStore := agentdata.NewStore(d.DB())
	platformStore := platformdata.NewStore(d.DB())
	t.Setenv("AGENT_SECRET_KEY", strings.Repeat("s", 32))
	sealer := llm.EnvSealer{Key: []byte(strings.Repeat("s", 32))}
	modelSvc := llm.NewService(platformStore, sealer, fgaAuthorizer, testGateway{}, llm.Config{BaseURL: "http://fixture.invalid"})
	if err := platformStore.UpsertModels(ctx, []domain.Model{{
		ModelKey: "fixture/model", Provider: "fixture", GatewayModelKey: "model",
		DisplayName: "Fixture", MaxOutputTokens: 8192, SupportsTools: true,
		Enabled: true, Curated: true,
	}}); err != nil {
		t.Fatalf("seed model: %v", err)
	}
	models, err := platformStore.ListModels(ctx, true)
	if err != nil || len(models) == 0 {
		t.Fatalf("load seeded model: %v", err)
	}
	testModelServiceID = 0
	testModelID = int64(models[0].ID)
	auditSvc := audit.NewService(platformStore)
	quotaSvc := app.NewQuotaService(agentStore, app.QuotaConfig{Timezone: "UTC"})
	quotaSvc.Authz = fgaAuthorizer
	quotaSvc.Audit = auditSvc
	mcpDiscovery := &agentmcp.Provider{
		Store: platformStore, Sealer: sealer,
		Guard: &agentmcp.Guard{AllowPrivate: true, AllowInsecure: true},
	}
	botStore := botdata.NewStore(d.DB())
	botSealer := botadapters.Sealer{Key: []byte(strings.Repeat("s", 32))}
	botGuard := &agentmcp.Guard{AllowPrivate: true, AllowInsecure: true}
	botSvc := botapp.NewService(botStore, fgaAuthorizer, botSealer, botGuard, auditSvc, testLogger)
	botAuth := botapp.NewAuthService(botStore, testLogger)
	botReader := &botadapters.Reader{Query: query}
	botSender := &botadapters.Sender{Messages: messageApp}
	botUsers := botadapters.Users{Repo: authRepo}
	botMembers := botadapters.GroupMembers{Repo: groupRepo}
	botDeliverySvc := botapp.NewDeliveryService(botStore, botReader, botUsers, botdelivery.NewSender(botGuard, testLogger), botSealer, testLogger)
	botDeliverySvc.Config = botapp.DeliveryConfig{MaxAttempts: 3, Jitter: false}
	botStreamSvc := botapp.NewStreamService(botStore, fgaAuthorizer, botstream.NewRedisStore(d.Redis), botSender, botadapters.Recipients{Groups: botMembers}, nil, testLogger)
	botHTTP := service.NewBotHTTP(botSvc, botAuth, botDeliverySvc, botStreamSvc, botReader, botSender, nil, testLogger)

	agentSvc := service.NewAgentHTTP(
		admin.NewAgentService(agentStore, platformStore, modelSvc, modelSvc, fgaAuthorizer, groupRepo, testUserDirectory{repo: authRepo}, auditSvc, testLogger),
		admin.NewRunService(agentStore, fgaAuthorizer, realtime.NewMemoryRunEventStore(), nil),
		admin.NewApprovalService(agentStore, fgaAuthorizer, auditSvc),
		admin.NewMCPService(platformStore, fgaAuthorizer, mcpDiscovery, sealer, auditSvc, testLogger),
		admin.NewSkillService(platformStore, fgaAuthorizer, auditSvc),
		modelSvc,
		quotaSvc,
		auditSvc,
		nil,
	)

	var worker *imconsumer.Worker
	var grpcCleanup func()
	if useKafka {
		grpcAddr, cleanupGRPC := setupGRPCPushServer(t, gwManager)
		grpcCleanup = cleanupGRPC
		rawPusher, err := realtime.NewGatewayPusher([]string{grpcAddr}, testLogger)
		if err != nil {
			t.Fatalf("failed to create gateway pusher: %v", err)
		}
		pushSvc := &imapp.PushService{
			Pusher: impush.NewPusher(rawPusher), GroupMembers: groupMembers, Bots: botIndex,
			Metrics: testMetrics, Log: log.NewHelper(testLogger),
		}
		consumer := kafkax.NewConsumer(kafkax.ConsumerConfig{
			Brokers: []string{testKafkaBroker}, Topic: kafkaTopic,
			GroupID:  fmt.Sprintf("test-msgworker-%d", time.Now().UnixNano()),
			MinBytes: 1, MaxBytes: 10e6, StartOffset: kafka.FirstOffset,
		}, nil, testLogger)
		worker = imconsumer.NewWorker(consumer, messageApp, pushSvc, nil, testLogger, testMetrics)
		worker.Store = storage
		worker.SetBotIndex(botIndex)
		worker.SetGroupMemberService(groupMembers)
		worker.SetConversationRepo(conversationRepo)
		go func() {
			if err := worker.Start(context.Background()); err != nil {
				t.Logf("msgworker start error: %v", err)
			}
		}()
		time.Sleep(500 * time.Millisecond)
	}

	confServer := &conf.Server{
		Http: &conf.Server_HTTP{Addr: testHTTPAddr},
		Grpc: &conf.Server_GRPC{Addr: "0.0.0.0:0"},
	}
	fileSvc := service.NewFileService(fileApp, testLogger)
	outboxSvc := &imapp.OutboxService{Repo: storage, Log: log.NewHelper(testLogger)}
	outboxAdmin := service.NewOutboxAdminHTTP(messagingim.NewOutboxAdmin(outboxSvc), fgaAuthorizer, auditSvc)
	hs := server.NewHTTPServer(confServer, authSvc, dispatchSvc, msgSvc, userSvc, conversationSvc, groupSvc, fileSvc, agentSvc, botHTTP, outboxAdmin, wsServer, authConf, nil, nil, testLogger, testMetrics)
	go func() {
		if err := hs.Start(ctx); err != nil {
			select {
			case <-ctx.Done():
			default:
				t.Logf("http server start/stop error: %v", err)
			}
		}
	}()
	if err := waitForServer(testBaseURL + "/v1/dispatch/gateway"); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	return &testServer{
		data: d, cleanup: cleanup, httpSrv: hs, gwManager: gwManager, gwSessionMgr: gwSessionMgr,
		redis: d.Redis, authConf: authConf, kafkaProducer: msgProducer, kafkaTopic: kafkaTopic,
		mongoDB: mongoDB, storage: storage, searchIndex: searchIndex, msgWorker: worker, messageApp: messageApp,
		query: query, groupRepo: groupRepo, botIndex: botIndex, fileSvc: fileSvc,
		grpcCleanup: grpcCleanup, openFGA: fgaAuthorizer, fgaCleanup: fgaCleanup,
		modelServiceID: 0, modelID: int64(models[0].ID), modelSvc: modelSvc,
		botStore: botStore, botDelivery: botDeliverySvc, botStreams: botStreamSvc,
	}
}

func (ts *testServer) stop() {
	if ts.msgWorker != nil {
		_ = ts.msgWorker.Stop(context.Background())
	}
	if ts.grpcCleanup != nil {
		ts.grpcCleanup()
	}
	if ts.httpSrv != nil {
		ts.httpSrv.Stop(context.Background())
	}
	if ts.fgaCleanup != nil {
		ts.fgaCleanup()
	}
	if ts.cleanup != nil {
		ts.cleanup()
	}
	if ts.mongoDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if client := ts.mongoDB.Client(); client != nil {
			_ = client.Disconnect(ctx)
		}
	}
}

func waitForServer(url string) error {
	for i := 0; i < 50; i++ {
		resp, err := httpClient.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server did not become ready in time")
}

// ---------- OpenFGA test helpers ----------

// waitForOpenFGA waits for the OpenFGA HTTP API to become ready.
func waitForOpenFGA(apiURL string) error {
	for i := 0; i < 100; i++ {
		resp, err := httpClient.Get(apiURL + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("openfga did not become ready in time")
}

// setupOpenFGA creates a fresh OpenFGA store with the embedded authorization
// model for a single test server instance, and returns the authorizer plus a
// cleanup that deletes the store again. The API token is read from
// OPENFGA_API_TOKEN, defaulting to the preshared key configured in
// docker-compose.test.yml, and exported so the authorizer built by
// authz.NewOpenFGA attaches the same credentials. While the test container
// still runs with authn=none the token is simply ignored; if the server ever
// rejects it with 401 we fall back to anonymous access (probed once, kept
// simple).
func setupOpenFGA(t *testing.T) (*authz.OpenFGA, func()) {
	t.Helper()
	if err := waitForOpenFGA(testOpenFGAURL); err != nil {
		t.Fatalf("openfga not ready: %v", err)
	}
	token := os.Getenv("OPENFGA_API_TOKEN")
	if token == "" {
		token = testOpenFGADefaultToken
	}
	if openFGAAcceptsToken(token) {
		os.Setenv("OPENFGA_API_TOKEN", token)
	} else {
		t.Logf("openfga rejected the configured token; continuing without credentials")
		os.Unsetenv("OPENFGA_API_TOKEN")
		token = ""
	}

	api := func(method, path string, body, out any) error {
		var reader io.Reader
		if body != nil {
			buf, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reader = bytes.NewReader(buf)
		}
		req, err := http.NewRequest(method, testOpenFGAURL+path, reader)
		if err != nil {
			return err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		resp, err := httpClient.Do(req.WithContext(ctx))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, payload)
		}
		if out != nil {
			return json.NewDecoder(resp.Body).Decode(out)
		}
		return nil
	}

	storeName := fmt.Sprintf("nonoka-it-%d", time.Now().UnixNano())
	var store struct {
		ID string `json:"id"`
	}
	if err := api(http.MethodPost, "/stores", map[string]string{"name": storeName}, &store); err != nil {
		t.Fatalf("create openfga store: %v", err)
	}
	model, err := authz.Model()
	if err != nil {
		t.Fatalf("load embedded authorization model: %v", err)
	}
	var written struct {
		AuthorizationModelID string `json:"authorization_model_id"`
	}
	if err := api(http.MethodPost, "/stores/"+store.ID+"/authorization-models", model, &written); err != nil {
		t.Fatalf("write openfga authorization model: %v", err)
	}
	fga, err := authz.NewOpenFGA(testOpenFGAURL, store.ID, written.AuthorizationModelID)
	if err != nil {
		t.Fatalf("create openfga authorizer: %v", err)
	}
	cleanup := func() {
		_ = api(http.MethodDelete, "/stores/"+store.ID, nil, nil)
	}
	return fga, cleanup
}

// openFGAAcceptsToken probes the store listing once with the given bearer
// token. A 401 means the server runs a different authn setup, so anonymous
// access is used instead.
func openFGAAcceptsToken(token string) bool {
	if token == "" {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, testOpenFGAURL+"/stores", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode != http.StatusUnauthorized
}

// httpPost sends a JSON POST request and unmarshals the response.
func httpPost(t *testing.T, url string, body interface{}) *http.Response {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("failed to encode request body: %v", err)
	}

	resp, err := httpClient.Post(url, "application/json", &buf)
	if err != nil {
		t.Fatalf("failed to POST %s: %v", url, err)
	}
	return resp
}

// httpPostAuth sends a JSON POST request with an Authorization bearer token.
func httpPatchAuth(t *testing.T, url, token string, body interface{}) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create patch request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch request failed: %v", err)
	}
	return resp
}

func httpDeleteAuth(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("create delete request failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	return resp
}

func httpPostAuth(t *testing.T, url, token string, body interface{}) *http.Response {
	return httpDoAuth(t, http.MethodPost, url, token, body)
}

// httpPutAuth sends a JSON PUT request with an Authorization bearer token.
func httpPutAuth(t *testing.T, url, token string, body interface{}) *http.Response {
	return httpDoAuth(t, http.MethodPut, url, token, body)
}

func httpDoAuth(t *testing.T, method, url, token string, body interface{}) *http.Response {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("failed to encode request body: %v", err)
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("failed to create %s request: %v", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("failed to %s %s: %v", method, url, err)
	}
	return resp
}

// httpGet sends a GET request with optional Authorization header.
func httpGet(t *testing.T, url, token string) *http.Response {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("failed to create GET request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("failed to GET %s: %v", url, err)
	}
	return resp
}

// assertStatusCode checks response status code.
func assertStatusCode(t *testing.T, resp *http.Response, expected int) {
	if resp.StatusCode != expected {
		var body map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		t.Fatalf("expected status %d, got %d, body: %+v", expected, resp.StatusCode, body)
	}
}

// ---------- WebSocket test helpers ----------

// wsConnect establishes a WebSocket connection to the test server.
func wsConnect(t *testing.T) *websocket.Conn {
	wsConn, _, err := websocket.DefaultDialer.Dial(testWSURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	return wsConn
}

// wsSendPacket marshals and sends a protobuf Packet over WebSocket.
func wsSendPacket(t *testing.T, wsConn *websocket.Conn, packet *v1.Packet) {
	data, err := proto.Marshal(packet)
	if err != nil {
		t.Fatalf("failed to marshal packet: %v", err)
	}
	if err := wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("failed to write websocket message: %v", err)
	}
}

// wsReadPacket reads and unmarshals a protobuf Packet from WebSocket.
func wsReadPacket(t *testing.T, wsConn *websocket.Conn, timeout time.Duration) *v1.Packet {
	if timeout > 0 {
		wsConn.SetReadDeadline(time.Now().Add(timeout))
		defer wsConn.SetReadDeadline(time.Time{})
	}
	_, data, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read websocket message: %v", err)
	}
	var packet v1.Packet
	if err := proto.Unmarshal(data, &packet); err != nil {
		t.Fatalf("failed to unmarshal packet: %v", err)
	}
	return &packet
}

// wsReadPacketOrNil reads a packet with timeout, returns nil on timeout.
func wsReadPacketOrNil(t *testing.T, wsConn *websocket.Conn, timeout time.Duration) *v1.Packet {
	if timeout > 0 {
		wsConn.SetReadDeadline(time.Now().Add(timeout))
		defer wsConn.SetReadDeadline(time.Time{})
	}
	_, data, err := wsConn.ReadMessage()
	if err != nil {
		return nil
	}
	var packet v1.Packet
	if err := proto.Unmarshal(data, &packet); err != nil {
		return nil
	}
	return &packet
}

// generateJWTToken creates a test JWT token for the given user_id.
func generateJWTToken(userID int64, secret string) string {
	token := jwt5.NewWithClaims(jwt5.SigningMethodHS256, jwt5.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	s, _ := token.SignedString([]byte(secret))
	return s
}

// registerAndLogin creates a user and returns their JWT token and userID.
func registerAndLogin(t *testing.T, username, password string) (token string, userID int64) {
	// Under concurrent CI load, register/login may hit transient 5xx (and a
	// timed-out register may still have committed, surfacing as 400 on retry)
	// or a 401 login right after a silently-failed register. Retry the whole
	// register+login cycle a few times instead of failing the test.
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}

		// Register; USERNAME_EXISTS (400) means a previous attempt persisted.
		respReg := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
			"username": username,
			"password": password,
		})
		respReg.Body.Close()
		if respReg.StatusCode >= 500 {
			continue
		}

		resp := httpPost(t, testBaseURL+"/v1/auth/login", map[string]string{
			"username": username,
			"password": password,
			"deviceId": "test-device",
		})
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		var reply v1.LoginReply
		decodeProtoJSON(t, resp.Body, &reply)
		resp.Body.Close()
		if reply.Token == "" || reply.UserId == 0 {
			t.Fatalf("login returned empty token or zero user_id")
		}
		return reply.Token, reply.UserId
	}
	t.Fatalf("registerAndLogin failed for %q after retries", username)
	return "", 0
}

// ---------- Kafka test helpers ----------

// waitForTopicReady waits until the topic exists, has partitions, and each partition has a leader.
func waitForTopicReady(broker, topic string) error {
	for i := 0; i < 100; i++ {
		conn, err := kafka.Dial("tcp", broker)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		partitions, err := conn.ReadPartitions(topic)
		conn.Close()
		if err != nil || len(partitions) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// Ensure each partition has a leader (broker is fully ready to accept writes)
		allReady := true
		for _, p := range partitions {
			if p.Leader.ID == 0 {
				allReady = false
				break
			}
		}
		if allReady {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("topic %s did not become ready in time", topic)
}

// waitForKafka waits for Kafka broker to become available.
func waitForKafka(broker string) error {
	for i := 0; i < 50; i++ {
		conn, err := kafka.Dial("tcp", broker)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("kafka broker did not become ready in time")
}

// recreateTopicStable deletes and recreates a topic like
// cleanupAndCreateTopic, then guards against the broker's asynchronous
// deletion: when a topic is deleted and immediately recreated with the same
// name, the broker can finish deleting the old incarnation after the new one
// was created, purging the live topic mid-test and breaking every subsequent
// produce with "Unknown Topic Or Partition". The purge is a one-shot event,
// so keeping the partitions alive through a short settle period proves the
// recreation is final.
func recreateTopicStable(broker, topic string, partitions int) error {
	const rounds = 3
	for round := 0; round < rounds; round++ {
		if err := cleanupAndCreateTopic(broker, topic, partitions); err != nil {
			return err
		}
		survived := true
		for i := 0; i < 10; i++ {
			time.Sleep(300 * time.Millisecond)
			conn, err := kafka.Dial("tcp", broker)
			if err != nil {
				return err
			}
			parts, err := conn.ReadPartitions(topic)
			conn.Close()
			if err != nil || len(parts) == 0 {
				survived = false
				break
			}
		}
		if survived {
			return nil
		}
	}
	return fmt.Errorf("topic %s was purged %d times after recreation", topic, rounds)
}

// cleanupAndCreateTopic deletes a Kafka topic if it exists and recreates it fresh.
// It waits for deletion to propagate before creating and then waits for the new
// topic to be fully visible, reducing metadata propagation races in a busy
// test suite.
func cleanupAndCreateTopic(broker, topic string, partitions int) error {
	conn, err := kafka.Dial("tcp", broker)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Try to delete the topic if it exists.
	_ = conn.DeleteTopics(topic)

	// Wait for deletion to propagate before recreating. Recreating while a
	// previous incarnation is still being removed can leave the topic in an
	// inconsistent state.
	for i := 0; i < 50; i++ {
		parts, err := conn.ReadPartitions(topic)
		if err != nil || len(parts) == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Create the topic fresh.
	if err := conn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     partitions,
		ReplicationFactor: 1,
	}); err != nil {
		return err
	}

	// Ensure the topic is actually visible and ready before returning.
	return waitForTopicReady(broker, topic)
}

// produceMessageWithRetry attempts to produce a message, retrying transient
// errors such as "Unknown Topic Or Partition" while Kafka metadata propagates.
// Newly created topics can take seconds to become fully writable, so keep a
// generous retry budget. It is safe for idempotent messages thanks to
// client_msg_id deduplication.
func produceMessageWithRetry(ctx context.Context, t *testing.T, producer *gateway.KafkaProducer, upstream *v1.UpstreamMessage) {
	t.Helper()
	const maxAttempts = 30
	for i := 0; i < maxAttempts; i++ {
		err := producer.Produce(ctx, upstream)
		if err == nil {
			return
		}
		errStr := err.Error()
		if i == maxAttempts-1 {
			t.Fatalf("failed to produce message after %d attempts: %v", maxAttempts, err)
		}
		// Retry only metadata propagation / transient errors.
		if !containsAny(errStr, []string{"Unknown Topic Or Partition", "Leader Not Available", "Not Leader For Partition"}) {
			t.Fatalf("failed to produce message: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// containsAny reports whether s contains any of the substrings.
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if bytes.Contains([]byte(s), []byte(sub)) {
			return true
		}
	}
	return false
}

// createKafkaReader creates a new Kafka reader using a consumer group.
// Since the topic is freshly created for each test, it will read all messages
// produced during the test from all partitions.
func createKafkaReader(broker, topic, groupID string) *kafka.Reader {
	// Append timestamp to groupID to avoid offset conflicts from previous test runs.
	uniqueGroupID := groupID + "-" + fmt.Sprintf("%d", time.Now().UnixNano())
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{broker},
		Topic:       topic,
		GroupID:     uniqueGroupID,
		MinBytes:    1,
		MaxBytes:    10e6,
		StartOffset: kafka.FirstOffset, // Topic is fresh, read all messages from beginning
	})
}

// consumeKafkaMessage reads a single message from the reader within the timeout.
func consumeKafkaMessage(t *testing.T, reader *kafka.Reader, timeout time.Duration) *kafka.Message {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("failed to read kafka message: %v", err)
	}
	return &msg
}

// consumeKafkaMessageOrNil reads a message with timeout, returns nil on timeout.
func consumeKafkaMessageOrNil(reader *kafka.Reader, timeout time.Duration) *kafka.Message {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		return nil
	}
	return &msg
}

// ---------- MsgWorker test helpers ----------

const (
	testMongoURI = "mongodb://127.0.0.1:27018/?replicaSet=rs0&directConnection=true"
	testMongoDB  = "nonoka_im_test"
)

// waitForMongo waits for MongoDB to become available.
func waitForMongo(uri string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("mongodb did not become ready in time")
		default:
		}
		client, err := mongo.Connect(options.Client().ApplyURI(uri).SetConnectTimeout(2 * time.Second))
		if err == nil {
			err = client.Ping(ctx, nil)
			if err == nil {
				_ = client.Disconnect(ctx)
				return nil
			}
			_ = client.Disconnect(ctx)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// setupMongoDB connects to the test MongoDB and returns the database.
func setupMongoDB(t *testing.T) *mongo.Database {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires mongodb")
	}
	if err := waitForMongo(testMongoURI); err != nil {
		t.Fatalf("mongodb not ready: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(testMongoURI))
	if err != nil {
		t.Fatalf("failed to connect to mongodb: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("failed to ping mongodb: %v", err)
	}

	db := client.Database(testMongoDB)

	// Clean collections for test isolation
	collections := []string{"messages", "inboxes", "topic_seqs"}
	for _, coll := range collections {
		if err := db.Collection(coll).Drop(ctx); err != nil {
			t.Logf("warning: failed to drop collection %s: %v", coll, err)
		}
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	})

	return db
}

// setupTestRedis connects to the test Redis and returns the client.
func setupTestRedis(t *testing.T) redis.UniversalClient {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires redis")
	}
	client := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6380",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis not ready: %v", err)
	}
	// Clean im:seq:* keys for test isolation
	keys, err := client.Keys(ctx, "im:seq:*").Result()
	if err != nil && err != redis.Nil {
		t.Logf("warning: failed to list redis seq keys: %v", err)
	} else if len(keys) > 0 {
		if err := client.Del(ctx, keys...).Err(); err != nil {
			t.Logf("warning: failed to clean redis seq keys: %v", err)
		}
	}
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}

// setupGRPCPushServer starts a minimal gRPC server with PushService.
// Returns the listener address and a cleanup function.
func setupGRPCPushServer(t *testing.T, manager *gateway.Manager) (string, func()) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := lis.Addr().String()

	pushSvc := service.NewPushService(manager, testLogger)
	grpcSrv := grpc.NewServer()
	v1.RegisterPushServiceServer(grpcSrv, pushSvc)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			// Expected on stop
		}
	}()

	cleanup := func() {
		grpcSrv.GracefulStop()
		lis.Close()
	}

	// Wait briefly for server to be ready
	time.Sleep(50 * time.Millisecond)

	return addr, cleanup
}

// countMongoDocs counts documents in a collection matching the filter.
func countMongoDocs(t *testing.T, coll *mongo.Collection, filter bson.M) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		t.Fatalf("failed to count documents: %v", err)
	}
	return count
}

// newTestIMWorker builds the consumer pipeline used by msgworker integration
// tests without starting a Kafka reader.
func newTestIMWorker(seqGen *seq.Generator, storage *mongostore.Store, pusher realtime.Pusher, conversations imapp.ConversationRepository, members imapp.GroupMemberReader, bots *imgroup.BotIndex, consumers ...*kafkax.Consumer) *imconsumer.Worker {
	var adapter imapp.Pusher
	if pusher != nil {
		adapter = impush.NewPusher(pusher)
	}
	var botDir imapp.BotDirectory
	if bots != nil {
		botDir = bots
	}
	var memberDir imapp.GroupMemberReader
	if members != nil {
		memberDir = members
	}
	messages := &imapp.MessageService{
		Writer: storage, IDs: seq.NewIDs(1), Seqs: seqGen,
		Conversations: conversations, GroupMembers: memberDir, Bots: botDir,
		Log: log.NewHelper(testLogger),
	}
	push := &imapp.PushService{Pusher: adapter, GroupMembers: memberDir, Bots: botDir, Log: log.NewHelper(testLogger)}
	var consumer *kafkax.Consumer
	if len(consumers) > 0 {
		consumer = consumers[0]
	}
	worker := imconsumer.NewWorker(consumer, messages, push, nil, testLogger)
	worker.Store = storage
	return worker
}

func boolPtr(v bool) *bool { return &v }

type testGateway struct{}

func (testGateway) CreateVirtualKey(_ context.Context, spec llm.VirtualKeySpec) (*llm.VirtualKey, error) {
	return &llm.VirtualKey{ID: "vk-" + spec.Name, Value: "sk-bf-test"}, nil
}
func (testGateway) UpdateVirtualKey(context.Context, string, llm.VirtualKeySpec, bool) error {
	return nil
}
func (testGateway) DeleteVirtualKey(context.Context, string) error { return nil }
func (testGateway) EnsureCustomer(context.Context, llm.CustomerSpec) (string, error) {
	return "customer-test", nil
}
func (testGateway) ListGatewayModels(context.Context, string) ([]llm.GatewayModel, error) {
	return []llm.GatewayModel{{ID: "fixture/model", NormalizedName: "Fixture", OwnedBy: "fixture"}}, nil
}
