// internal/user/adapter/out/persistence/user_repository.go

// Package persistence is the user context's secondary ("driven") Postgres
// adapter.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"pet-messenger/internal/user/domain"
)

// UserRepository implements this context's own out.UserStorePort. It also
// exposes two public methods that are NOT part of any port declared inside
// internal/user/application — Create and GetProfilesByIDs — because no use
// case inside this context calls either. They exist solely so that
// auth's own out-port and conversation's own ProfileBatchViewPort (each
// defined in their own packages) can be satisfied structurally. See
// docs/superpowers/specs/2026-10-13-user-clean-hexagonal-cqrs-design.md.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository constructs the Postgres-backed UserRepository.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create persists a new user. Not part of out.UserStorePort — see the type
// doc comment; consumed by auth's own out-port.
func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	const q = `
		INSERT INTO users (id, username, display_name, avatar_url, password_hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, q, u.ID, u.Username, u.DisplayName, u.AvatarURL, u.PasswordHash, u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrUsernameTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetByID implements out.UserStorePort.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return r.scanOne(ctx, `SELECT id, username, display_name, avatar_url, password_hash, created_at FROM users WHERE id = $1`, id)
}

// GetByUsername implements out.UserStorePort.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	return r.scanOne(ctx, `SELECT id, username, display_name, avatar_url, password_hash, created_at FROM users WHERE username = $1`, username)
}

// Update implements out.UserStorePort.
func (r *UserRepository) Update(ctx context.Context, u *domain.User) error {
	const q = `UPDATE users SET username = $2, display_name = $3, avatar_url = $4 WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, u.ID, u.Username, u.DisplayName, u.AvatarURL)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrUsernameTaken
		}
		return fmt.Errorf("update user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update user rows affected: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepository) scanOne(ctx context.Context, query string, arg any) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, query, arg)
	var u domain.User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}

// GetProfilesByIDs resolves a batch of user ids to their public profiles.
// Not part of out.UserStorePort — see the type doc comment; consumed by
// conversation's own ProfileBatchViewPort. Returned in no particular
// order — callers must index the result by Profile.ID, never by position
// relative to ids. An id with no matching row is simply absent from the
// result, never an error: the project has no user-deletion path today, so
// this is expected to be unreachable in practice, not a case callers need
// to branch on. ids are bound as a []string (not []uuid.UUID) cast to
// uuid[] in SQL — a plain, driver-agnostic array-parameter technique that
// doesn't depend on pgx's native UUID-array encoding.
func (r *UserRepository) GetProfilesByIDs(ctx context.Context, ids []uuid.UUID) ([]*domain.Profile, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = id.String()
	}
	const q = `SELECT id, username, display_name, avatar_url FROM users WHERE id = ANY($1::uuid[])`
	rows, err := r.db.QueryContext(ctx, q, idStrs)
	if err != nil {
		return nil, fmt.Errorf("query profiles: %w", err)
	}
	defer rows.Close()

	var out []*domain.Profile
	for rows.Next() {
		var p domain.Profile
		if err := rows.Scan(&p.ID, &p.Username, &p.DisplayName, &p.AvatarURL); err != nil {
			return nil, fmt.Errorf("scan profile: %w", err)
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate profile rows: %w", err)
	}
	return out, nil
}
