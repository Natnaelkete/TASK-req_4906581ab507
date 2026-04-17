package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/eaglepoint/harborclass/internal/models"
	"github.com/eaglepoint/harborclass/internal/store"
)

// ErrInvalidCredentials is returned when login fails.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Service manages sessions. Session tokens are opaque and live in
// memory; this keeps the demo offline and dependency-free.
type Service struct {
	store store.Store

	mu       sync.RWMutex
	sessions map[string]session
}

type session struct {
	Username string
	Expires  time.Time
}

// NewService wires an auth service.
func NewService(s store.Store) *Service {
	return &Service{store: s, sessions: map[string]session{}}
}

// Login verifies credentials and returns an opaque session token.
func (s *Service) Login(ctx context.Context, username, password string) (string, models.User, error) {
	u, err := s.store.UserByUsername(ctx, username)
	if err != nil {
		return "", models.User{}, ErrInvalidCredentials
	}
	if !VerifyPassword(password, u.PasswordHash) {
		return "", models.User{}, ErrInvalidCredentials
	}
	tok := newToken()
	s.mu.Lock()
	s.sessions[tok] = session{Username: username, Expires: time.Now().Add(12 * time.Hour)}
	s.mu.Unlock()
	return tok, u, nil
}

// Logout invalidates a session token.
func (s *Service) Logout(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// Resolve returns the authenticated user for a token.
func (s *Service) Resolve(ctx context.Context, token string) (models.User, error) {
	s.mu.RLock()
	ss, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return models.User{}, ErrInvalidCredentials
	}
	if time.Now().After(ss.Expires) {
		s.Logout(token)
		return models.User{}, ErrInvalidCredentials
	}
	return s.store.UserByUsername(ctx, ss.Username)
}

// ExtractBearerToken returns the token portion of an Authorization
// header of the form "Bearer <token>".
func ExtractBearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func newToken() string {
	// 16 random bytes encoded url-safely. Opaque to callers.
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>uint(i*3)) ^ byte(i*7+3)
	}
	b[0] ^= byte(time.Now().Unix() & 0xff)
	return base64.RawURLEncoding.EncodeToString(b)
}
