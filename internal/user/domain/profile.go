// internal/user/domain/profile.go
package domain

import "github.com/google/uuid"

// Profile is the read-only subset of a User exposed to other bounded
// contexts that need to display who a user is without depending on the
// full User aggregate (which also carries PasswordHash). It is returned
// only by UserRepository.GetProfilesByIDs, a method with no port inside
// this package (nothing in user's own application layer looks up a batch
// of profiles; only internal/conversation's own ProfileBatchViewPort does,
// see docs/superpowers/specs/2026-10-13-user-clean-hexagonal-cqrs-design.md).
type Profile struct {
	ID          uuid.UUID
	Username    string
	DisplayName string
	AvatarURL   string
}
