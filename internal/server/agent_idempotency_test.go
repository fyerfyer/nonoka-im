package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentErrorEncoderSkipsCommittedResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents", nil)
	agentErrorEncoder(rec, req, errAgentResponseCommitted)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("committed response was overwritten: code=%d body=%q", rec.Code, rec.Body.String())
	}
}
