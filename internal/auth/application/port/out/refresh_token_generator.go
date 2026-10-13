package out

// RefreshTokenGenerator is implemented by
// internal/auth/adapter/out/refreshtoken.Generator.
type RefreshTokenGenerator interface {
	Generate() (plain string, hash string, err error)
	Hash(plain string) string
}
