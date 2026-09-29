package session

import (
	"database/sql"
	"errors"

	"eigenshare/internal/database"
	"eigenshare/internal/timestamp"
)

type SessionRepository struct {
	db *database.Database
}

func NewSessionRepository(db *database.Database) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Save(s Session) error {
	var expiresAt any
	if s.ExpiresAt != nil {
		expiresAt = s.ExpiresAt.String()
	}

	_, err := r.db.DB.Exec(
		"INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		s.Token, s.UserID, s.CreatedAt.String(), expiresAt,
	)
	return err
}

func (r *SessionRepository) Get(token string) (*Session, error) {
	var s Session
	var expiresAt sql.NullString

	err := r.db.DB.QueryRow(
		"SELECT token, user_id, created_at, expires_at FROM sessions WHERE token = ?",
		token,
	).Scan(&s.Token, &s.UserID, &s.CreatedAt, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
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
	_, err := r.db.DB.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}
