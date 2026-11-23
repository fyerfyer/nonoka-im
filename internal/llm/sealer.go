package llm

import (
	"encoding/base64"
	"fmt"

	"nonoka-im/internal/agent/credential"
)

// EnvSealer implements Sealer using the shared agent credential key.
type EnvSealer struct{ Key []byte }

func (s EnvSealer) Seal(plain string) (string, error) {
	if len(s.Key) < 32 {
		return "", fmt.Errorf("AGENT_SECRET_KEY must contain at least 32 bytes")
	}
	sealed, err := credential.Seal(s.Key, []byte(plain))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (s EnvSealer) Open(sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	if len(s.Key) < 32 {
		return "", fmt.Errorf("AGENT_SECRET_KEY must contain at least 32 bytes")
	}
	plain, err := credential.Open(s.Key, raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
