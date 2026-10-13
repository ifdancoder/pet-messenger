// internal/user/application/command/command_test_helpers_test.go
package command_test

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/google/uuid"

	"pet-messenger/internal/user/domain"
)

// fakeUserStore is an in-memory out.UserStorePort.
type fakeUserStore struct {
	byID      map[uuid.UUID]*domain.User
	byName    map[string]*domain.User
	updateErr error
}

func newFakeUserStore(users ...*domain.User) *fakeUserStore {
	s := &fakeUserStore{byID: map[uuid.UUID]*domain.User{}, byName: map[string]*domain.User{}}
	for _, u := range users {
		cp := *u
		s.byID[u.ID] = &cp
		s.byName[u.Username] = &cp
	}
	return s
}

func (s *fakeUserStore) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if u, ok := s.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (s *fakeUserStore) GetByUsername(_ context.Context, username string) (*domain.User, error) {
	if u, ok := s.byName[username]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (s *fakeUserStore) Update(_ context.Context, u *domain.User) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	existing, ok := s.byID[u.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if taken, ok := s.byName[u.Username]; ok && taken.ID != u.ID {
		return domain.ErrUsernameTaken
	}
	delete(s.byName, existing.Username)
	cp := *u
	s.byID[u.ID] = &cp
	s.byName[u.Username] = &cp
	return nil
}

var errFakeAvatarNotFound = errors.New("fake: avatar not found")

// fakeAvatarStore is an in-memory out.AvatarStorePort.
type fakeAvatarStore struct {
	data        map[uuid.UUID][]byte
	contentType map[uuid.UUID]string
	putErr      error
}

func newFakeAvatarStore() *fakeAvatarStore {
	return &fakeAvatarStore{data: map[uuid.UUID][]byte{}, contentType: map[uuid.UUID]string{}}
}

func (s *fakeAvatarStore) Put(_ context.Context, userID uuid.UUID, contentType string, data io.Reader, _ int64) error {
	if s.putErr != nil {
		return s.putErr
	}
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	s.data[userID] = b
	s.contentType[userID] = contentType
	return nil
}

func (s *fakeAvatarStore) Get(_ context.Context, userID uuid.UUID) (io.ReadCloser, string, error) {
	b, ok := s.data[userID]
	if !ok {
		return nil, "", errFakeAvatarNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), s.contentType[userID], nil
}
