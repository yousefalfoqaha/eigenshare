package user

import (
	"database/sql"
	"errors"

	"eigenshare/backend/infra"
)

type UserRepository struct {
	storage *infra.Database
}

func NewUserRepository(db *infra.Database) *UserRepository {
	return &UserRepository{storage: db}
}

func (r *UserRepository) Save(u User) error {
	_, err := r.storage.DB.Exec(
		"INSERT INTO users (id, username, password_hash, is_admin, created_at) VALUES (?, ?, ?, ?, ?)",
		u.ID, u.Username, u.PasswordHash, u.IsAdmin, u.CreatedAt.String(),
	)
	return err
}

func (r *UserRepository) Update(u User) error {
	_, err := r.storage.DB.Exec(
		"UPDATE users SET username = ?, password_hash = ?, is_admin = ? WHERE id = ?",
		u.Username, u.PasswordHash, u.IsAdmin, u.ID,
	)
	return err
}

func (r *UserRepository) GetByUsername(username string) (*User, error) {
	var u User
	err := r.storage.DB.QueryRow(
		"SELECT id, username, password_hash, is_admin, created_at FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) GetByID(id string) (*User, error) {
	var u User
	err := r.storage.DB.QueryRow(
		"SELECT id, username, password_hash, is_admin, created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) GetAll() ([]*User, error) {
	rows, err := r.storage.DB.Query(
		"SELECT id, username, password_hash, is_admin, created_at FROM users ORDER BY created_at",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

func (r *UserRepository) Delete(id string) error {
	_, err := r.storage.DB.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (r *UserRepository) Count() (int, error) {
	var count int
	err := r.storage.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (r *UserRepository) CountAdmins() (int, error) {
	var count int
	err := r.storage.DB.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = 1").Scan(&count)
	return count, err
}
