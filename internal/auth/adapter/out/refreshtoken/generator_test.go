// internal/auth/adapter/out/refreshtoken/generator_test.go
package refreshtoken_test

import (
	"testing"

	"pet-messenger/internal/auth/adapter/out/refreshtoken"
)

func TestGenerator_Generate(t *testing.T) {
	g := refreshtoken.NewGenerator()

	plain1, hash1, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	plain2, hash2, err := g.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if plain1 == plain2 {
		t.Error("expected two calls to produce distinct plaintext tokens")
	}
	if hash1 == hash2 {
		t.Error("expected two calls to produce distinct hashes")
	}
	if g.Hash(plain1) != hash1 {
		t.Error("Hash(plain) must be deterministic and match what Generate returned")
	}
}
