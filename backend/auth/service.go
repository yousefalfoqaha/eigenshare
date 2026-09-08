package auth

import (
	"time"

	"eigenshare/backend/user"

	"github.com/google/uuid"
)

const sessionExpiryMinutes = 60 * 24 * 7

type AuthService struct {
	userSvc     *user.UserService
	sessionRepo *SessionRepository
}

func NewAuthService(userSvc *user.UserService, sessionRepo *SessionRepository) *AuthService {
	return &AuthService{userSvc: userSvc, sessionRepo: sessionRepo}
}

func (s *AuthService) Login(c LoginRequest) (*Session, error) {
	u, err := s.userSvc.GetUserByUsername(c.Username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}

	ok, err := u.VerifyPassword(c.Password)
	if err != nil || !ok {
		return nil, ErrInvalidCredentials
	}

	session, err := NewSession(uuid.New().String(), u.ID, sessionExpiryMinutes)
	if err != nil {
		return nil, err
	}

	if err := s.sessionRepo.Save(*session); err != nil {
		return nil, err
	}

	return session, nil
}

func (s *AuthService) Logout(token string) error {
	if token == "" {
		return nil
	}
	return s.sessionRepo.Delete(token)
}

func (s *AuthService) ValidateSession(token string) (*user.User, error) {
	session, err := s.sessionRepo.Get(token)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}

	if session.ExpiresAt != nil && time.Time(*session.ExpiresAt).Before(time.Now().UTC()) {
		_ = s.sessionRepo.Delete(token)
		return nil, ErrSessionNotFound
	}

	u, err := s.userSvc.GetUser(session.UserID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrSessionNotFound
	}

	return u, nil
}
