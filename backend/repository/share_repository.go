package repository

import (
	"database/sql"
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"eigenshare/backend/timestamp"
	"fmt"
	"strings"
)

type ShareRepository struct {
	storage *storage.DatabaseStorage
}

func NewShareRepository(db *storage.DatabaseStorage) *ShareRepository {
	return &ShareRepository{storage: db}
}

func (r *ShareRepository) GetAll() ([]share.ShareSummary, error) {
	var summaries []share.ShareSummary
	query := `
		SELECT 
			s.id, 
			s.handle,
			COUNT(f.` + "`order`" + `) AS totalFiles, 
			COALESCE(SUM(f.expected_size), 0) AS totalSize
		FROM share s
		LEFT JOIN file f ON s.id = f.share_id
		GROUP BY s.id, s.handle`

	rows, err := r.storage.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var s share.ShareSummary
		if err := rows.Scan(&s.ID, &s.Handle, &s.TotalFiles, &s.TotalSize); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

func (r *ShareRepository) Get(id string) (*share.Share, error) {
	var sh share.Share
	var expiresAt sql.NullString

	row := r.storage.DB.QueryRow("SELECT id, handle, created_at, expires_at FROM share WHERE id = ?", id)
	if err := row.Scan(&sh.ID, &sh.Handle, &sh.CreatedAt, &expiresAt); err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		var ea timestamp.Timestamp
		if err := ea.Scan(expiresAt.String); err != nil {
			return nil, err
		}
		sh.ExpiresAt = &ea
	}

	files, err := r.getAllFilesByShareID(sh.ID)
	if err != nil {
		return nil, err
	}

	sh.Files = files

	return &sh, nil
}

func (r *ShareRepository) MakeTransaction(o func(tx *sql.Tx) error) error {
	tx, err := r.storage.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := o(tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ShareRepository) Save(s share.Share, tx *sql.Tx) (*share.Share, error) {
	var exists bool

	err := tx.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM share WHERE handle = ? AND id != ?)",
		s.Handle, s.ID,
	).Scan(&exists)
	if err != nil {
		return nil, err
	}

	if exists {
		return nil, fmt.Errorf("%w: %s", share.ErrHandleExists, s.Handle)
	}

	var expiresAt any
	if s.ExpiresAt != nil {
		expiresAt = s.ExpiresAt.String()
	}

	_, err = tx.Exec(
		"INSERT INTO share (id, handle, created_at, expires_at) VALUES (?, ?, ?, ?)",
		s.ID, s.Handle, s.CreatedAt.String(), expiresAt,
	)
	if err != nil {
		return nil, err
	}

	for _, f := range s.Files {
		_, err := tx.Exec(
			"INSERT INTO file (id, name, expected_size, `order`, share_id) VALUES (?, ?, ?, ?, ?)",
			f.ID, f.Name, f.ExpectedSize, f.Order, s.ID,
		)
		if err != nil {
			return nil, err
		}
	}

	return &s, nil
}

func (r *ShareRepository) Delete(ids []string, tx *sql.Tx) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := "DELETE FROM share WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	_, err := tx.Exec(query, args...)
	return err
}

func (r *ShareRepository) getAllFilesByShareID(id string) ([]share.File, error) {
	var files []share.File
	rows, err := r.storage.DB.Query(
		"SELECT id, name, expected_size, `order` FROM file WHERE share_id = ? ORDER BY `order`", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var file share.File
		if err := rows.Scan(&file.ID, &file.Name, &file.ExpectedSize, &file.Order); err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

func (r *ShareRepository) GetExpiredIDs() ([]string, error) {
	now := timestamp.Now()
	rows, err := r.storage.DB.Query(
		"SELECT id FROM share WHERE expires_at IS NOT NULL AND expires_at < ?", now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		ids = append(ids, id)
	}

	return ids, rows.Err()
}
