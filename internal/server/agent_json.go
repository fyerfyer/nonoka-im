package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

var numericJSON = regexp.MustCompile(`^[0-9]+$`)

// numericStringFields are 64-bit proto fields that protojson normally encodes
// as JSON strings. The existing SDK and web client use numbers for these
// fields, so the agent runtime HTTP surface keeps the old JSON contract while
// gRPC clients still receive native uint64/int64 values.
var numericStringFields = map[string]struct{}{
	"id": {}, "runId": {}, "agentId": {}, "agentVersion": {}, "sessionId": {},
	"msgId": {}, "topicSeq": {}, "replyMsgId": {}, "approverId": {}, "decidedBy": {},
	"triggerMsgId": {}, "botUserId": {}, "actorUserId": {}, "seq": {},
	"approvalId": {}, "nextSeq": {}, "inputTokens": {}, "outputTokens": {},
	"totalTokens": {}, "timestampMs": {}, "version": {}, "sourceRevision": {},
	"publishedBy": {}, "modelId": {}, "revision": {}, "serverId": {}, "skillId": {},
	"currentVersion": {}, "ownerUserId": {}, "latestVersion": {}, "createdBy": {},
	"toolsRevision": {}, "updatedBy": {}, "dailyTokens": {}, "runs": {},
	"total": {}, "createdAt": {},
}

// opaqueJSONFields contain arbitrary user/audit data. Their nested keys may
// legitimately be numeric strings ("id": "007"), so the numeric compatibility
// rewrite must not descend into them.
var opaqueJSONFields = map[string]struct{}{
	"before": {}, "after": {}, "inputSchema": {},
}

// agentNumericAPIPaths lists the generated Agent HTTP surfaces that keep the
// pre-proto JSON contract. Bot management is excluded because its proto
// contract already uses string IDs.
func agentNumericAPIPath(path string) bool {
	for _, prefix := range []string{
		"/v1/agent-runs", "/v1/agent-approvals", "/v1/agent-tools", "/v1/agents",
		"/v1/mcp", "/v1/skills", "/v1/models",
		"/v1/admin/quotas", "/v1/admin/usage", "/v1/admin/audit-events", "/v1/admin/agent-outbox",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func agentAPIPath(path string) bool {
	return agentNumericAPIPath(path) || path == "/v1/bots" || strings.HasPrefix(path, "/v1/bots/") ||
		strings.HasPrefix(path, "/v1/admin/")
}

func agentNumericResponseEncoder(w http.ResponseWriter, r *http.Request, v any) error {
	if !agentNumericAPIPath(r.URL.Path) {
		return khttp.DefaultResponseEncoder(w, r, v)
	}
	msg, ok := v.(proto.Message)
	if !ok {
		return khttp.DefaultResponseEncoder(w, r, v)
	}
	data, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	decoded = convertNumericStrings(decoded)
	data, err = json.Marshal(decoded)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	return err
}

// agentErrorEncoder keeps the established {code,message,retryable,...}
// envelope for generated Agent/Bot HTTP handlers. Native Kratos errors use
// numeric code + reason, which would otherwise be a breaking SDK/Web change.
func agentErrorEncoder(w http.ResponseWriter, r *http.Request, err error) {
	if !agentAPIPath(r.URL.Path) {
		khttp.DefaultErrorEncoder(w, r, err)
		return
	}
	se := kerrors.FromError(err)
	status := int(se.Code)
	if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	code := se.Reason
	if code == "" {
		code = "HTTP_" + strconv.Itoa(status)
	}
	retryAfter := time.Duration(0)
	if raw := se.Metadata["retry-after-ms"]; raw != "" {
		if ms, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && ms > 0 {
			retryAfter = time.Duration(ms) * time.Millisecond
		}
	}
	requestID := se.Metadata["requestId"]
	if requestID == "" {
		requestID = r.Header.Get("X-Request-Id")
	}
	body := map[string]any{
		"code": code, "message": se.Message,
		"retryable": status == 408 || status == 429 || status >= 500,
	}
	if retryAfter > 0 {
		body["retryAfterMs"] = retryAfter.Milliseconds()
	}
	if requestID != "" {
		body["requestId"] = requestID
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func convertNumericStrings(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if item == nil {
				// protojson EmitUnpopulated renders absent messages as null;
				// the pre-proto DTOs omitted them instead.
				delete(typed, key)
				continue
			}
			if _, opaque := opaqueJSONFields[key]; opaque {
				continue
			}
			if text, ok := item.(string); ok && numericJSON.MatchString(text) {
				if _, wanted := numericStringFields[key]; wanted {
					typed[key] = json.Number(text)
					continue
				}
			}
			typed[key] = convertNumericStrings(item)
		}
		return typed
	case []any:
		for i := range typed {
			typed[i] = convertNumericStrings(typed[i])
		}
		return typed
	default:
		return value
	}
}
