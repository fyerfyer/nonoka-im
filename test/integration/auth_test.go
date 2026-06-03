package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	v1 "nonoka-im/api/im/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func decodeProtoJSON(t *testing.T, body io.Reader, msg proto.Message) {
	b, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	// Use protojson because Kratos HTTP returns camelCase field names
	// matching protobuf JSON encoding rules.
	if err := protojson.Unmarshal(b, msg); err != nil {
		t.Fatalf("failed to decode protojson response: %v\nbody: %s", err, string(b))
	}
}

func TestAuth_Register_Success(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	resp := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
		"username": "alice",
		"password": "123456",
	})
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	var reply v1.RegisterReply
	decodeProtoJSON(t, resp.Body, &reply)
	if reply.UserId == 0 {
		t.Fatalf("expected non-zero user_id, got %d", reply.UserId)
	}
}

func TestAuth_Register_DuplicateUsername(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// First registration should succeed
	resp1 := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
		"username": "bob",
		"password": "123456",
	})
	resp1.Body.Close()
	assertStatusCode(t, resp1, http.StatusOK)

	// Second registration with same username should fail
	resp2 := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
		"username": "bob",
		"password": "anotherpassword",
	})
	defer resp2.Body.Close()

	assertStatusCode(t, resp2, http.StatusBadRequest)

	var errBody map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&errBody); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errBody["reason"] != "USERNAME_EXISTS" {
		t.Fatalf("expected reason USERNAME_EXISTS, got %v", errBody["reason"])
	}
}

func TestAuth_Register_MissingFields(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	resp := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
		"username": "",
		"password": "",
	})
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusBadRequest)
}

func TestAuth_Login_Success(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Register first
	respReg := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
		"username": "charlie",
		"password": "123456",
	})
	respReg.Body.Close()
	assertStatusCode(t, respReg, http.StatusOK)

	// Login
	resp := httpPost(t, testBaseURL+"/v1/auth/login", map[string]string{
		"username": "charlie",
		"password": "123456",
		"deviceId": "web-001",
	})
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	var reply v1.LoginReply
	decodeProtoJSON(t, resp.Body, &reply)
	if reply.UserId == 0 {
		t.Fatalf("expected non-zero user_id, got %d", reply.UserId)
	}
	if reply.Token == "" {
		t.Fatalf("expected non-empty token")
	}
}

func TestAuth_Login_InvalidPassword(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Register
	respReg := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
		"username": "dave",
		"password": "123456",
	})
	respReg.Body.Close()
	assertStatusCode(t, respReg, http.StatusOK)

	// Login with wrong password
	resp := httpPost(t, testBaseURL+"/v1/auth/login", map[string]string{
		"username": "dave",
		"password": "wrongpassword",
	})
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusUnauthorized)

	var errBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	// Should return vague error to prevent user enumeration
	if errBody["reason"] != "AUTH_FAILED" {
		t.Fatalf("expected reason AUTH_FAILED, got %v", errBody["reason"])
	}
}

func TestAuth_Login_NonExistentUser(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	resp := httpPost(t, testBaseURL+"/v1/auth/login", map[string]string{
		"username": "nobody",
		"password": "123456",
	})
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusUnauthorized)

	var errBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errBody["reason"] != "AUTH_FAILED" {
		t.Fatalf("expected reason AUTH_FAILED for non-existent user, got %v", errBody["reason"])
	}
}

func TestDispatch_Gateway_PublicAccess(t *testing.T) {
	ts := setupTestServer(t, false)
	defer ts.stop()

	// Dispatch gateway should be publicly accessible without JWT
	resp := httpGet(t, testBaseURL+"/v1/dispatch/gateway", "")
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	var reply v1.GetGatewayReply
	decodeProtoJSON(t, resp.Body, &reply)
	if reply.GatewayUrl == "" {
		t.Fatalf("expected non-empty gateway_url")
	}
}