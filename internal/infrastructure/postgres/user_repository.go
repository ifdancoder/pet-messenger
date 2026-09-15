// internal/infrastructure/postgres/user_repository.go
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"pet-messenger/internal/domain/user"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) user.Repository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	const q = `
		INSERT INTO users (id, username, display_name, avatar_url, password_hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.ExecContext(ctx, q, u.ID, u.Username, u.DisplayName, u.AvatarURL, u.PasswordHash, u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return user.ErrUsernameTaken
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	return r.scanOne(ctx, `SELECT id, username, display_name, avatar_url, password_hash, created_at FROM users WHERE id = $1`, id)
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	return r.scanOne(ctx, `SELECT id, username, display_name, avatar_url, password_hash, created_at FROM users WHERE username = $1`, username)
}

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	const q = `UPDATE users SET username = $2, display_name = $3, avatar_url = $4 WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, u.ID, u.Username, u.DisplayName, u.AvatarURL)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return user.ErrUsernameTaken
		}
		return fmt.Errorf("update user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update user rows affected: %w", err)
	}
	if n == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (r *UserRepository) scanOne(ctx context.Context, query string, arg any) (*user.User, error) {
	row := r.db.QueryRowContext(ctx, query, arg)
	var u user.User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}
