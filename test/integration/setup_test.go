package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	v1 "nonoka-im/api/im/v1"
	"nonoka-im/internal/biz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/gateway"
	"nonoka-im/internal/msgworker"
	"nonoka-im/internal/metrics"
	"nonoka-im/internal/server"
	"nonoka-im/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const (
	testHTTPAddr = "0.0.0.0:18000"
	testBaseURL  = "http://127.0.0.1:18000"
	testWSURL    = "ws://127.0.0.1:18000/ws"

	testKafkaBroker = "127.0.0.1:9092"
	testKafkaTopic  = "im-messages-test"
)

var (
	testLogger = log.NewStdLogger(os.Stdout)
	httpClient = &http.Client{Timeout: 5 * time.Second}
)

// testServer holds all dependencies for integration tests.
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
	storage        *msgworker.MessageStorage
	msgWorker      *msgworker.MsgWorker
	grpcCleanup    func()
}

// setupTestServer bootstraps a full HTTP server against the test database.
// If useKafka is true, it connects to the test Kafka instance; otherwise it uses a no-op producer.
func setupTestServer(t *testing.T, useKafka bool) *testServer {
	if testing.Short() {
		t.Skip("skipping integration test in short mode: requires postgres, redis, mongodb and kafka")
	}
	ctx := context.Background()

	// 1. Data layer with both PostgreSQL and Redis
	confData := &conf.Data{
		Database: &conf.Data_Database{
			Driver: "postgres",
			Source: "host=127.0.0.1 user=postgres password=root dbname=nonoka_im_test port=5433 sslmode=disable TimeZone=Asia/Shanghai",
		},
		Redis: &conf.Data_Redis{
			Addr: "127.0.0.1:6380",
		},
	}
	d, cleanup, err := data.NewData(confData)
	if err != nil {
		t.Fatalf("failed to create data layer: %v", err)
	}

	// 2. Clean tables before test
	if err := d.CleanTestData(); err != nil {
		t.Fatalf("failed to clean test data: %v", err)
	}

	// 2.5 Clean Redis test keys
	if d.Redis != nil {
		if err := d.Redis.FlushDB(ctx).Err(); err != nil {
			t.Fatalf("failed to flush redis test db: %v", err)
		}
	}

	// 3. MongoDB for message storage (offline pull support)
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(testMongoURI))
	if err != nil {
		t.Fatalf("failed to connect to mongodb: %v", err)
	}
	mongoDB := mongoClient.Database(testMongoDB)
	// Clean collections for test isolation. This must run BEFORE
	// EnsureIndexes below: dropping a collection also drops its indexes,
	// including the unique indexes that make message deduplication
	// race-safe across parallel Kafka partitions.
	for _, coll := range []string{"messages", "inboxes", "topic_seqs", "mention_inboxes"} {
		_ = mongoDB.Collection(coll).Drop(ctx)
	}
	storage := msgworker.NewMessageStorage(mongoDB, testLogger)
	if err := storage.EnsureIndexes(ctx); err != nil {
		t.Fatalf("failed to ensure mongodb indexes: %v", err)
	}

	// 4. Auth config (bcrypt at min cost: dozens of concurrent register/login
	// calls in tests would otherwise starve the CPU and flake under CI load)
	authConf := &conf.Auth{
		JwtSecret:  "test-jwt-secret-do-not-use-in-production",
		TokenTtl:   durationpb.New(time.Hour),
		BcryptCost: int32(bcrypt.MinCost),
	}

	// 5. Biz layer
	authRepo := data.NewAuthRepo(d, testLogger)
	authUC := biz.NewAuthUsecase(authRepo, authConf)

	// 6. Service layer
	authSvc := service.NewAuthService(authUC)
	dispatchConf := &conf.Dispatch{
		Strategy: "consistent_hash",
	}
	dispatchSvc := service.NewDispatchService(d.Redis, dispatchConf, testLogger)

	// 7. Gateway layer
	gwManager := gateway.NewManager(testLogger)
	gwSessionMgr := gateway.NewSessionManager(d.Redis, "test-gateway-node")

	var msgProducer gateway.MessageProducer
	var kafkaTopic string
	if useKafka {
		if err := waitForKafka(testKafkaBroker); err != nil {
			t.Fatalf("kafka not ready: %v", err)
		}
		kafkaTopic = testKafkaTopic
		// Recreate topic fresh for each test to ensure isolation, and verify
		// the recreation survives the broker's asynchronous deletion of the
		// previous incarnation (see recreateTopicStable).
		if err := recreateTopicStable(testKafkaBroker, kafkaTopic, 3); err != nil {
			t.Fatalf("failed to create kafka topic: %v", err)
		}
		kafkaCfg := gateway.KafkaConfig{
			Brokers:     []string{testKafkaBroker},
			Topic:       kafkaTopic,
			BatchSize:   10,
			MaxAttempts: 30, // extra retries to tolerate test-topic metadata propagation
		}
		msgProducer = gateway.NewKafkaProducer(kafkaCfg, testLogger)
	} else {
		msgProducer = gateway.NewNoopProducer()
	}

	gwHandler := gateway.NewHandler(gwManager, gwSessionMgr, msgProducer, storage, []byte(authConf.JwtSecret), gateway.HeartbeatConfig{
		Interval: 30 * time.Second,
		Timeout:  90 * time.Second,
	}, testLogger)
	// Integration tests intentionally exercise rapid/burst traffic, so the
	// per-connection publish rate limiter is disabled here. It is covered by
	// unit tests and enabled with real defaults in the wire assembly.
	gwHandler.SetMessageRateLimit(0, 0)
	wsServer := gateway.NewWebSocketServer(gwHandler, testLogger, 60*time.Second, 10*time.Second, nil)

	// 7. Message service (with producer for HTTP fallback)
	msgSvc := service.NewMessageService(storage, msgProducer, testLogger)
	userSvc := service.NewUserService(authUC)
	conversationSvc := service.NewConversationService(data.NewConversationRepo(d, testLogger))

	// 7.5 Start msgworker and gRPC push server when using Kafka
	var worker *msgworker.MsgWorker
	var grpcCleanup func()
	if useKafka {
		// Start gRPC push server for msgworker -> gateway push
		grpcAddr, cleanupGRPC := setupGRPCPushServer(t, gwManager)
		grpcCleanup = cleanupGRPC

		// Create msgworker dependencies
		seqGen := msgworker.NewSeqGenerator(d.Redis, testLogger)
		snowflake := msgworker.NewSnowflake(1)
		pusher, err := msgworker.NewGatewayPusher([]string{grpcAddr}, testLogger)
		if err != nil {
			t.Fatalf("failed to create gateway pusher: %v", err)
		}

		consumerCfg := msgworker.KafkaConsumerConfig{
			Brokers:     []string{testKafkaBroker},
			Topic:       kafkaTopic,
			GroupID:     "test-msgworker-" + fmt.Sprintf("%d", time.Now().UnixNano()),
			MinBytes:    1,
			MaxBytes:    10e6,
			StartOffset: kafka.FirstOffset,
		}
		consumer := msgworker.NewKafkaConsumer(consumerCfg, nil, testLogger)
		worker = msgworker.NewMsgWorker(consumer, seqGen, snowflake, storage, pusher, testLogger)

		// Use PostgreSQL-backed group membership with Redis cache.
		groupMemberRepo := data.NewGroupMemberRepo(d, testLogger)
		groupMemberSvc := msgworker.NewPersistentGroupMemberService(groupMemberRepo, d.Redis, testLogger)
		worker.SetGroupMemberService(groupMemberSvc)

		// Wire conversation summary repository for tests.
		worker.SetConversationRepo(data.NewConversationRepo(d, testLogger))

		consumer.SetHandler(worker.HandleMessage)

		go func() {
			if err := worker.Start(context.Background()); err != nil {
				t.Logf("msgworker start error: %v", err)
			}
		}()

		// Wait for consumer to be ready
		time.Sleep(500 * time.Millisecond)
	}

	// 8. HTTP server with WebSocket handler
	confServer := &conf.Server{
		Http: &conf.Server_HTTP{Addr: testHTTPAddr},
		Grpc: &conf.Server_GRPC{Addr: "0.0.0.0:0"},
	}
	testMetrics := metrics.NewMetrics()
	// Rate limiting is disabled in the integration harness: several existing
	// tests deliberately hammer login/register and rapid WS publishes. The
	// limiter logic itself is covered by unit tests.
	hs := server.NewHTTPServer(confServer, authSvc, dispatchSvc, msgSvc, userSvc, conversationSvc, nil, wsServer, authConf, nil, nil, testLogger, testMetrics)

	// 8. Start HTTP server in background
	go func() {
		if err := hs.Start(ctx); err != nil {
			// Server stopped gracefully on cleanup, ignore expected error
			select {
			case <-ctx.Done():
				return
			default:
				t.Logf("http server start/stop error: %v", err)
			}
		}
	}()

	// 9. Wait for server readiness
	if err := waitForServer(testBaseURL + "/v1/dispatch/gateway"); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	return &testServer{
		data:          d,
		cleanup:       cleanup,
		httpSrv:       hs,
		gwManager:     gwManager,
		gwSessionMgr:  gwSessionMgr,
		redis:         d.Redis,
		authConf:      authConf,
		kafkaProducer: msgProducer,
		kafkaTopic:    kafkaTopic,
		mongoDB:       mongoDB,
		storage:       storage,
		msgWorker:     worker,
		grpcCleanup:   grpcCleanup,
	}
}

func (ts *testServer) stop() {
	if ts.msgWorker != nil {
		_ = ts.msgWorker.Stop()
	}
	if ts.grpcCleanup != nil {
		ts.grpcCleanup()
	}
	if ts.httpSrv != nil {
		ts.httpSrv.Stop(context.Background())
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
	testMongoURI = "mongodb://127.0.0.1:27018"
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
