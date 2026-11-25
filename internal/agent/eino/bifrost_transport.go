package eino

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nonoka-im/internal/agent/domain"
	"nonoka-im/internal/agent/port"
)

// bifrostTransport authenticates inference with x-bf-vk and translates
// Bifrost's 402/429 refusals into typed budget/rate-limit errors before they
// reach the OpenAI adapter.
type bifrostTransport struct {
	base http.RoundTripper
	vk   string
}

func (t *bifrostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("x-bf-vk", t.vk)
	clone.Header.Del("Authorization")
	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusPaymentRequired && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 400 {
		return resp, nil
	}
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	bodyType, message := parseBifrostError(payload)
	switch resp.StatusCode {
	case http.StatusPaymentRequired:
		fallback := "Agent budget is exhausted."
		if message != "" {
			fallback = "Agent budget is exhausted: " + message
		}
		return nil, &domain.BudgetError{Message: fallback}
	case http.StatusTooManyRequests:
		retry := parseRateLimitReset(message)
		if retry <= 0 {
			retry = 60 * time.Second
		}
		if message == "" {
			message = "The model provider is rate limiting requests."
		}
		return nil, &domain.RateLimitError{RetryAfter: retry, Message: message}
	case http.StatusRequestTimeout, http.StatusConflict:
		return nil, fmt.Errorf("bifrost status %d: %s", resp.StatusCode, firstNonEmpty(message, bodyType))
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("bifrost status %d: %s", resp.StatusCode, firstNonEmpty(message, bodyType))
	}
	if bodyType == "model_blocked" || bodyType == "provider_blocked" || bodyType == "virtual_key_required" || bodyType == "access_blocked" || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, &port.Error{Class: port.ClassInvalidConfig, Err: fmt.Errorf("bifrost: %s", firstNonEmpty(message, bodyType))}
	}
	return nil, &port.Error{Class: port.ClassModel, Err: fmt.Errorf("bifrost status %d: %s", resp.StatusCode, firstNonEmpty(message, bodyType))}
}

func parseBifrostError(payload []byte) (string, string) {
	var body struct {
		Type  string `json:"type"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &body) == nil {
		return body.Type, body.Error.Message
	}
	return "", strings.TrimSpace(string(payload))
}

var rateResetPattern = regexp.MustCompile(`resets every ([0-9]+)(ms|s|m|h|d)`)

func parseRateLimitReset(message string) time.Duration {
	m := rateResetPattern.FindStringSubmatch(message)
	if len(m) != 3 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0
	}
	switch m[2] {
	case "ms":
		return time.Duration(n) * time.Millisecond
	case "s":
		return time.Duration(n) * time.Second
	case "m":
		return time.Duration(n) * time.Minute
	case "h":
		return time.Duration(n) * time.Hour
	case "d":
		return time.Duration(n) * 24 * time.Hour
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
