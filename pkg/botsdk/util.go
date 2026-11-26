package botsdk

import (
	"crypto/rand"
	"encoding/hex"
)

func randHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "000000000000"
	}
	return hex.EncodeToString(buf)
}
