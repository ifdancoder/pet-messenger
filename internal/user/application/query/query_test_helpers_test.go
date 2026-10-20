// internal/user/application/query/query_test_helpers_test.go
package query_test

import (
	"bytes"
	"context"
	"io"

	"github.com/google/uuid"

	"pet-messenger/internal/user/domain"
)

// fakeUserStore is an in-memory out.UserStorePort, a separate copy from
// internal/user/application/command's fake of the same shape, since
// command and query are different packages and this file's identifiers
// are not importable across packages.
type fakeUserStore struct {
	byName map[string]*domain.User
}

func newFakeUserStore(users ...*domain.User) *fakeUserStore {
	s := &fakeUserStore{byName: map[string]*domain.User{}}
	for _, u := range users {
		cp := *u
		s.byName[u.Username] = &cp
	}
	return s
}

func (s *fakeUserStore) GetByID(context.Context, uuid.UUID) (*domain.User, error) {
	return nil, domain.ErrNotFound // unused by this package's handlers
}

func (s *fakeUserStore) GetByUsername(_ context.Context, username string) (*domain.User, error) {
	if u, ok := s.byName[username]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (s *fakeUserStore) Update(context.Context, *domain.User) error {
	return nil // unused by this package's handlers
}

// fakeAvatarStore is an in-memory out.AvatarStorePort.
type fakeAvatarStore struct {
	data        map[uuid.UUID][]byte
	contentType map[uuid.UUID]string
}

func newFakeAvatarStore() *fakeAvatarStore {
	return &fakeAvatarStore{data: map[uuid.UUID][]byte{}, contentType: map[uuid.UUID]string{}}
}

func (s *fakeAvatarStore) put(userID uuid.UUID, contentType string, data []byte) {
	s.data[userID] = data
	s.contentType[userID] = contentType
}

func (s *fakeAvatarStore) Put(_ context.Context, userID uuid.UUID, contentType string, data io.Reader, _ int64) error {
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	s.put(userID, contentType, b)
	return nil
}

func (s *fakeAvatarStore) Get(_ context.Context, userID uuid.UUID) (io.ReadCloser, string, error) {
	b, ok := s.data[userID]
	if !ok {
		return nil, "", domain.ErrNotFound // any error is enough for this package's handlers
	}
	return io.NopCloser(bytes.NewReader(b)), s.contentType[userID], nil
}
