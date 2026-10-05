package user

import (
	"errors"
	"fmt"

	"eigenshare/internal/security"

	"github.com/google/uuid"
)

type UserService struct {
	userRepo *UserRepository
}

func NewUserService(userRepo *UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

func (s *UserService) CreateUser(req CreateRequest) (*User, error) {
	existing, err := s.userRepo.GetByUsername(req.Username)
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUsernameTaken
	}

	if len(req.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters: %w", security.ErrInvalidPassword)
	}
	if len(req.Password) > 72 {
		return nil, fmt.Errorf("password must be at most 72 characters: %w", security.ErrInvalidPassword)
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	u, err := NewUser(uuid.New().String(), req.Username, hash, req.IsAdmin)
	if err != nil {
		return nil, err
	}

	if err := s.userRepo.Save(*u); err != nil {
		return nil, err
	}

	return u, nil
}

func (s *UserService) UpdateUser(id, actorID string, req UpdateRequest) (*User, error) {
	u, err := s.userRepo.Get(id)
	if err != nil {
		return nil, err
	}

	if id == actorID && u.IsAdmin && !req.IsAdmin {
		return nil, ErrCannotDemoteSelf
	}

	if req.Username != u.Username {
		if err := validateUsername(req.Username); err != nil {
			return nil, err
		}
		existing, err := s.userRepo.GetByUsername(req.Username)
		if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
		if existing != nil {
			return nil, ErrUsernameTaken
		}
		u.Username = req.Username
	}

	if req.Password != "" {
		if len(req.Password) < 8 {
			return nil, fmt.Errorf("password must be at least 8 characters: %w", security.ErrInvalidPassword)
		}
		if len(req.Password) > 72 {
			return nil, fmt.Errorf("password must be at most 72 characters: %w", security.ErrInvalidPassword)
		}
		hash, err := security.HashPassword(req.Password)
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

func (s *UserService) DeleteUser(id, actorID string) error {
	if id == actorID {
		return ErrCannotDeleteSelf
	}

	target, err := s.userRepo.Get(id)
	if err != nil {
		return err
	}

	if target.IsAdmin {
		admins, err := s.userRepo.CountAdmins()
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrCannotDeleteLastAdmin
		}
	}

	return s.userRepo.Delete(id)
}

func (s *UserService) GetUser(id string) (*User, error) {
	u, err := s.userRepo.Get(id)
	if err != nil {
		return nil, err
	}

	return u, nil
}

func (s *UserService) GetUserByUsername(username string) (*User, error) {
	return s.userRepo.GetByUsername(username)
}

func (s *UserService) GetAllUsers() ([]*User, error) {
	return s.userRepo.GetAll()
}

func (s *UserService) CountUsers() (int, error) {
	return s.userRepo.Count()
}

func (s *UserService) SeedAdmin() error {
	count, err := s.userRepo.Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := security.HashPassword("admin")
	if err != nil {
		return err
	}

	u, err := NewUser(uuid.New().String(), "admin", hash, true)
	if err != nil {
		return err
	}

	return s.userRepo.Save(*u)
}
