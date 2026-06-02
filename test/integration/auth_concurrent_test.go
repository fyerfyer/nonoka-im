package integration

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

// TestAuth_Register_Concurrent_DuplicateUsername verifies that concurrent
// registration attempts with the same username are handled safely.
func TestAuth_Register_Concurrent_DuplicateUsername(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.stop()

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var successCount int32
	var conflictCount int32
	var otherErrorCount int32

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()

			resp := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
				"username": "concurrent-user",
				"password": "123456",
			})
			defer resp.Body.Close()

			switch resp.StatusCode {
			case http.StatusOK:
				atomic.AddInt32(&successCount, 1)
			case http.StatusBadRequest:
				var body map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&body)
				if body["reason"] == "USERNAME_EXISTS" {
					atomic.AddInt32(&conflictCount, 1)
				} else {
					atomic.AddInt32(&otherErrorCount, 1)
					t.Logf("unexpected bad request reason: %v", body["reason"])
				}
			default:
				atomic.AddInt32(&otherErrorCount, 1)
				t.Logf("unexpected status code: %d", resp.StatusCode)
			}
		}()
	}

	wg.Wait()

	// Exactly one registration should succeed, the rest should get USERNAME_EXISTS
	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful registration, got %d", successCount)
	}
	if conflictCount != concurrency-1 {
		t.Fatalf("expected %d USERNAME_EXISTS responses, got %d", concurrency-1, conflictCount)
	}
	if otherErrorCount != 0 {
		t.Fatalf("expected 0 other errors, got %d", otherErrorCount)
	}

	t.Logf("concurrent test result: success=%d, conflict=%d", successCount, conflictCount)
}

// TestAuth_Login_Concurrent_SameUser verifies that concurrent login attempts
// with valid credentials all succeed and return valid JWT tokens.
func TestAuth_Login_Concurrent_SameUser(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.stop()

	// Register user first
	respReg := httpPost(t, testBaseURL+"/v1/auth/register", map[string]string{
		"username": "concurrent-login-user",
		"password": "123456",
	})
	respReg.Body.Close()
	assertStatusCode(t, respReg, http.StatusOK)

	const concurrency = 10
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var successCount int32
	var failCount int32

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()

			resp := httpPost(t, testBaseURL+"/v1/auth/login", map[string]string{
				"username": "concurrent-login-user",
				"password": "123456",
			})
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var body map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&body)
				if body["token"] != nil && body["token"] != "" {
					atomic.AddInt32(&successCount, 1)
				} else {
					atomic.AddInt32(&failCount, 1)
					t.Logf("login succeeded but token is empty")
				}
			} else {
				atomic.AddInt32(&failCount, 1)
				t.Logf("login failed with status: %d", resp.StatusCode)
			}
		}()
	}

	wg.Wait()

	if successCount != concurrency {
		t.Fatalf("expected %d successful logins, got %d (failed: %d)", concurrency, successCount, failCount)
	}

	t.Logf("concurrent login test result: success=%d, fail=%d", successCount, failCount)
}