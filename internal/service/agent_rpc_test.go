package service

import (
	"errors"
	"testing"

	"nonoka-im/internal/agent/app/admin"
	"nonoka-im/internal/httpapi"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestAgentRPCErrorPreservesHTTPStatus(t *testing.T) {
	err := agentRPCError(httpapi.NotFound("audit event not found"))
	se := kerrors.FromError(err)
	if se.Code != 404 || se.Reason != "NOT_FOUND" {
		t.Fatalf("not found mapped to %d/%s", se.Code, se.Reason)
	}

	err = agentRPCError(admin.ErrInvalidInput)
	se = kerrors.FromError(err)
	if se.Code != 400 || se.Reason != "INVALID_ARGUMENT" {
		t.Fatalf("invalid input mapped to %d/%s", se.Code, se.Reason)
	}

	err = agentRPCError(errors.New("boom"))
	se = kerrors.FromError(err)
	if se.Code != 500 {
		t.Fatalf("internal error mapped to %d", se.Code)
	}
}
