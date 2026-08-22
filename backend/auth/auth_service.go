package auth

import (
	"eigenshare/backend/user"
	"errors"
	"time"

	"github.com/google/uuid"
)

const sessionExpiryMinutes = 60 * 24 * 7

const (
	seedAdminUsername = "admin"
	seedAdminPassword = "admin"
)

type AuthService struct {
	userRepo    *user.UserRepository
	sessionRepo *user.SessionRepository
}

func NewAuthService(userRepo *user.UserRepository, sessionRepo *user.SessionRepository) *AuthService {
	return &AuthService{userRepo: userRepo, sessionRepo: sessionRepo}
}

func (s *AuthService) SeedAdmin() error {
	count, err := s.userRepo.Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := HashPassword(seedAdminPassword)
	if err != nil {
		return err
	}

	u, err := user.NewUser(uuid.New().String(), seedAdminUsername, hash, true)
	if err != nil {
		return err
	}

	return s.userRepo.Save(*u)
}

func (s *AuthService) Login(c Credentials) (*user.Session, error) {
	u, err := s.userRepo.GetByUsername(c.Username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, user.ErrInvalidCredentials
	}

	ok, err := VerifyPassword(c.Password, u.PasswordHash)
	if err != nil || !ok {
		return nil, user.ErrInvalidCredentials
	}

	session, err := user.NewSession(uuid.New().String(), u.ID, sessionExpiryMinutes)
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
		return nil, user.ErrSessionNotFound
	}

	if session.ExpiresAt != nil && time.Time(*session.ExpiresAt).Before(time.Now().UTC()) {
		_ = s.sessionRepo.Delete(token)
		return nil, user.ErrSessionNotFound
	}

	u, err := s.userRepo.GetByID(session.UserID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, user.ErrSessionNotFound
	}

	return u, nil
}

func (s *AuthService) CreateUser(req user.CreateRequest) (*user.User, error) {
	existing, err := s.userRepo.GetByUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, user.ErrUsernameTaken
	}

	if len(req.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	u, err := user.NewUser(uuid.New().String(), req.Username, hash, req.IsAdmin)
	if err != nil {
		return nil, err
	}

	if err := s.userRepo.Save(*u); err != nil {
		return nil, err
	}

	return u, nil
}

func (s *AuthService) UpdateUser(id, actorID string, req user.UpdateRequest) (*user.User, error) {
	u, err := s.userRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, user.ErrUserNotFound
	}

	if id == actorID && u.IsAdmin && !req.IsAdmin {
		return nil, user.ErrCannotDemoteSelf
	}

	if req.Username != u.Username {
		existing, err := s.userRepo.GetByUsername(req.Username)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, user.ErrUsernameTaken
		}
		u.Username = req.Username
	}

	if req.Password != "" {
		if len(req.Password) < 8 {
			return nil, errors.New("password must be at least 8 characters")
		}
		hash, err := HashPassword(req.Password)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = hash
	}

	u.IsAdmin = req.IsAdmin

	if err := s.userRepo.Update(*u); err != nil {
		return nil, err
	}

	return u, nil
}

func (s *AuthService) DeleteUser(id, actorID string) error {
	if id == actorID {
		return user.ErrCannotDeleteSelf
	}

	target, err := s.userRepo.GetByID(id)
	if err != nil {
		return err
	}
	if target == nil {
		return user.ErrUserNotFound
	}

	if target.IsAdmin {
		admins, err := s.userRepo.CountAdmins()
		if err != nil {
			return err
		}
		if admins <= 1 {
			return user.ErrCannotDeleteLastAdmin
		}
	}

	return s.userRepo.Delete(id)
}

func (s *AuthService) GetUser(id string) (*user.User, error) {
	u, err := s.userRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, user.ErrUserNotFound
	}
	return u, nil
}

func (s *AuthService) GetAllUsers() ([]*user.User, error) {
	return s.userRepo.GetAll()
}
