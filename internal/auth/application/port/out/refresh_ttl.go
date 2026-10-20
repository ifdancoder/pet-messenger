package out

import "time"

// RefreshTTL is the maximum lifetime of a refresh token since it was last
// issued or rotated, injected by internal/fx from config.Config.JWT.RefreshTTL.
// Not a port: a plain configuration value RefreshUseCase and fx both need
// to agree on the type of.
type RefreshTTL time.Duration
