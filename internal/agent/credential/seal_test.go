package credential

import (
	"bytes"
	"testing"
)

func testKey() []byte { return bytes.Repeat([]byte("k"), 32) }

func TestSealOpenRoundTrip(t *testing.T) {
	sealed, err := Seal(testKey(), []byte("sk-deepseek-example"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(sealed, []byte("deepseek")) {
		t.Fatalf("ciphertext leaks plaintext")
	}
	opened, err := Open(testKey(), sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(opened) != "sk-deepseek-example" {
		t.Fatalf("round trip mismatch: %q", opened)
	}
}

func TestSealOpenTampered(t *testing.T) {
	sealed, err := Seal(testKey(), []byte("secret"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	sealed[len(sealed)-1] ^= 0xff
	if _, err := Open(testKey(), sealed); err == nil {
		t.Fatalf("tampered ciphertext opened without error")
	}
}

func TestSealOpenWrongKey(t *testing.T) {
	sealed, err := Seal(testKey(), []byte("secret"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := Open(bytes.Repeat([]byte("x"), 32), sealed); err == nil {
		t.Fatalf("opened with wrong key")
	}
}

func TestSealShortKeyRejected(t *testing.T) {
	if _, err := Seal([]byte("short"), []byte("x")); err == nil {
		t.Fatalf("short key accepted")
	}
}
