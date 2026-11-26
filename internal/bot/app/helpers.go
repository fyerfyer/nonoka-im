package app

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"nonoka-im/internal/bot/domain"
)

func newWebhookSecret() (string, error) {
	buf := make([]byte, webhookSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "whsec_" + base64.StdEncoding.EncodeToString(buf), nil
}

func secretPrefix(secret string) string {
	if len(secret) <= 14 {
		return secret
	}
	return secret[:14] + "..."
}

func validateName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > maxBotNameBytes {
		return "", fmt.Errorf("%w: name is required and must be at most %d bytes", ErrInvalidInput, maxBotNameBytes)
	}
	return name, nil
}

func normalizeUsername(raw string) (username string, generated bool, err error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		username, err = randomUsername()
		return username, true, err
	}
	if len(raw) < 3 || len(raw) > 32 {
		return "", false, fmt.Errorf("%w: username must be 3-32 characters", ErrInvalidInput)
	}
	for i, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		case (r == '_' || r == '-') && i > 0:
		default:
			return "", false, fmt.Errorf("%w: username may only contain lowercase letters, digits, underscore or hyphen", ErrInvalidInput)
		}
	}
	return raw, false, nil
}

func randomUsername() (string, error) {
	suffix, err := randomHex(4)
	if err != nil {
		return "", err
	}
	return "bot_" + suffix, nil
}

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func normalizeWebhookEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return []string{domain.EventMessageMention, domain.EventMessageDirect}, nil
	}
	out := make([]string, 0, len(events))
	seen := map[string]struct{}{}
	for _, raw := range events {
		event := strings.TrimSpace(raw)
		switch event {
		case domain.EventMessageMention, domain.EventMessageDirect:
		default:
			return nil, fmt.Errorf("%w: unsupported webhook event", ErrInvalidInput)
		}
		if _, ok := seen[event]; ok {
			continue
		}
		seen[event] = struct{}{}
		out = append(out, event)
	}
	return out, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrWebhookAbsent) {
		return ErrNotFound
	}
	return err
}
