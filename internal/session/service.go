package session

import (
	"time"

	"eigenshare/internal/security"
	"eigenshare/internal/user"

	"github.com/google/uuid"
)

const sessionExpiryMinutes = 60 * 24 * 7

type SessionService struct {
	userSvc     *user.UserService
	sessionRepo *SessionRepository
}

func NewSessionService(userSvc *user.UserService, sessionRepo *SessionRepository) *SessionService {
	return &SessionService{userSvc: userSvc, sessionRepo: sessionRepo}
}

func (s *SessionService) Login(c LoginRequest) (*Session, error) {
	u, err := s.userSvc.GetUserByUsername(c.Username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}

	if !security.VerifyPassword(u.PasswordHash, c.Password) {
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

func (s *SessionService) Logout(token string) error {
	if token == "" {
		return nil
	}
	return s.sessionRepo.Delete(token)
}

func (s *SessionService) Validate(token string) (*user.User, error) {
	session, err := s.sessionRepo.Get(token)
	if err != nil {
		return nil, err
	}

	if session.ExpiresAt != nil && time.Time(*session.ExpiresAt).Before(time.Now().UTC()) {
		_ = s.sessionRepo.Delete(token)
		return nil, ErrSessionNotFound
	}

	u, err := s.userSvc.GetUser(session.UserID)
	if err != nil {
		return nil, err
	}

	return u, nil
}
