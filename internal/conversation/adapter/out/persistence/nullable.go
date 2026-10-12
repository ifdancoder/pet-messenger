// internal/conversation/adapter/out/persistence/nullable.go
package persistence

import "github.com/google/uuid"

// nullableUUID converts a possibly-nil *uuid.UUID into a database/sql exec
// argument: nil becomes SQL NULL, otherwise the dereferenced value.
func nullableUUID(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return *id
}
