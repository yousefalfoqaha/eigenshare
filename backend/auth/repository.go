package auth

import (
	"database/sql"
	"errors"

	"eigenshare/backend/infra"
	"eigenshare/backend/timestamp"
)

type SessionRepository struct {
	storage *infra.Database
}

func NewSessionRepository(db *infra.Database) *SessionRepository {
	return &SessionRepository{storage: db}
}

func (r *SessionRepository) Save(s Session) error {
	var expiresAt any
	if s.ExpiresAt != nil {
		expiresAt = s.ExpiresAt.String()
	}

	_, err := r.storage.DB.Exec(
		"INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		s.Token, s.UserID, s.CreatedAt.String(), expiresAt,
	)
	return err
}

func (r *SessionRepository) Get(token string) (*Session, error) {
	var s Session
	var expiresAt sql.NullString

	err := r.storage.DB.QueryRow(
		"SELECT token, user_id, created_at, expires_at FROM sessions WHERE token = ?",
		token,
	).Scan(&s.Token, &s.UserID, &s.CreatedAt, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if expiresAt.Valid {
		var ea timestamp.Timestamp
		if err := ea.Scan(expiresAt.String); err != nil {
			return nil, err
		}
		s.ExpiresAt = &ea
	}

	return &s, nil
}

func (r *SessionRepository) Delete(token string) error {
	_, err := r.storage.DB.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}
