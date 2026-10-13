package out

// PasswordHasher is implemented by internal/auth/adapter/out/bcrypt.Hasher.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hash, password string) error
}
