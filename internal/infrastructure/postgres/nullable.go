// internal/infrastructure/postgres/nullable.go
package postgres

import "github.com/google/uuid"

// nullableUUID adapts an optional UUID for binding to a nullable uuid column:
// a nil pointer becomes SQL NULL, a non-nil pointer its value.
func nullableUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}
