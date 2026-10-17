package user

import (
	"database/sql"
	"errors"

	"eigenshare/internal/database"
)

type UserRepository struct {
	db *database.Database
}

func NewUserRepository(db *database.Database) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Save(u User) error {
	_, err := r.db.DB.Exec(
		"INSERT INTO users (id, username, password_hash, is_admin, created_at, version) VALUES (?, ?, ?, ?, ?, ?)",
		u.ID, u.Username, u.PasswordHash, u.IsAdmin, u.CreatedAt, u.Version,
	)
	return err
}

func (r *UserRepository) Update(u *User) (*User, error) {
	result, err := r.db.DB.Exec(
		"UPDATE users SET username = ?, password_hash = ?, is_admin = ?, version = version + 1 WHERE id = ? AND version = ?",
		u.Username, u.PasswordHash, u.IsAdmin, u.ID, u.Version,
	)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected != 1 {
		return nil, ErrVersionConflict
	}

	u.Version++
	return u, nil
}

func (r *UserRepository) GetByUsername(username string) (*User, error) {
	var u User
	err := r.db.DB.QueryRow(
		"SELECT id, username, password_hash, is_admin, created_at, version FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt, &u.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.CreatedAt = u.CreatedAt.UTC()
	return &u, nil
}

func (r *UserRepository) Get(id string) (*User, error) {
	var u User
	err := r.db.DB.QueryRow(
		"SELECT id, username, password_hash, is_admin, created_at, version FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt, &u.Version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.CreatedAt = u.CreatedAt.UTC()
	return &u, nil
}

func (r *UserRepository) GetAll() ([]*User, error) {
	rows, err := r.db.DB.Query(
		"SELECT id, username, password_hash, is_admin, created_at, version FROM users ORDER BY created_at",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt, &u.Version); err != nil {
			return nil, err
		}
		u.CreatedAt = u.CreatedAt.UTC()
		users = append(users, &u)
	}
	return users, rows.Err()
}

func (r *UserRepository) Delete(id string) error {
	_, err := r.db.DB.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (r *UserRepository) Count() (int, error) {
	var count int
	err := r.db.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (r *UserRepository) CountAdmins() (int, error) {
	var count int
	err := r.db.DB.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = 1").Scan(&count)
	return count, err
}
