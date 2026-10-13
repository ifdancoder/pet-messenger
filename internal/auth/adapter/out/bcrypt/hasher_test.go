// internal/auth/adapter/out/bcrypt/hasher_test.go
package bcrypt_test

import (
	"testing"

	"pet-messenger/internal/auth/adapter/out/bcrypt"
)

func TestHasher_HashAndVerify(t *testing.T) {
	h := bcrypt.NewHasher()

	hash, err := h.Hash("correct-password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "correct-password" {
		t.Fatal("hash must not equal the plaintext password")
	}

	if err := h.Verify(hash, "correct-password"); err != nil {
		t.Errorf("Verify with correct password: %v", err)
	}
	if err := h.Verify(hash, "wrong-password"); err == nil {
		t.Error("expected an error verifying with the wrong password")
	}
}
