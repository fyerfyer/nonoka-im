package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"nonoka-im/internal/biz"
	"nonoka-im/internal/conf"
	"nonoka-im/internal/data"
	"nonoka-im/internal/server"
	"nonoka-im/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
)

const (
	testHTTPAddr = "0.0.0.0:18000"
	testBaseURL  = "http://127.0.0.1:18000"
)

var (
	testLogger = log.NewStdLogger(os.Stdout)
	httpClient = &http.Client{Timeout: 5 * time.Second}
)

// testServer holds all dependencies for integration tests.
type testServer struct {
	data    *data.Data
	cleanup func()
	httpSrv *khttp.Server
}

// setupTestServer bootstraps a full HTTP server against the test database.
func setupTestServer(t *testing.T) *testServer {
	ctx := context.Background()

	// 1. Data layer
	confData := &conf.Data{
		Database: &conf.Data_Database{
			Driver: "postgres",
			Source: "host=127.0.0.1 user=postgres password=root dbname=nonoka_im_test port=5433 sslmode=disable TimeZone=Asia/Shanghai",
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

	// 3. Auth config
	authConf := &conf.Auth{
		JwtSecret: "test-jwt-secret-do-not-use-in-production",
		TokenTtl:  durationpb.New(time.Hour),
	}

	// 4. Biz layer
	authRepo := data.NewAuthRepo(d, testLogger)
	authUC := biz.NewAuthUsecase(authRepo, authConf)

	// 5. Service layer
	authSvc := service.NewAuthService(authUC)
	dispatchSvc := service.NewDispatchService()

	// 6. HTTP server
	confServer := &conf.Server{
		Http: &conf.Server_HTTP{Addr: testHTTPAddr},
		Grpc: &conf.Server_GRPC{Addr: "0.0.0.0:0"},
	}
	hs := server.NewHTTPServer(confServer, authSvc, dispatchSvc, authConf, testLogger)

	// 7. Start HTTP server in background
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

	// 8. Wait for server readiness
	if err := waitForServer(testBaseURL + "/v1/dispatch/gateway"); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	return &testServer{
		data:    d,
		cleanup: cleanup,
		httpSrv: hs,
	}
}

func (ts *testServer) stop() {
	if ts.httpSrv != nil {
		ts.httpSrv.Stop(context.Background())
	}
	if ts.cleanup != nil {
		ts.cleanup()
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