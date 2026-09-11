package share

import (
	"database/sql"
	"fmt"

	"eigenshare/internal/infra"
	"eigenshare/internal/timestamp"
	"github.com/google/uuid"
)

type ShareRepository struct {
	storage *infra.Database
}

func NewShareRepository(db *infra.Database) *ShareRepository {
	return &ShareRepository{storage: db}
}

func (r *ShareRepository) GetAll() ([]ShareSummary, error) {
	var summaries []ShareSummary
	query := `
		SELECT 
			s.id, 
			s.handle,
			s.expires_at,
			COUNT(f.id) AS totalFiles,
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
		var s ShareSummary
		if err := rows.Scan(&s.ID, &s.Handle, &s.ExpiresAt, &s.TotalFiles, &s.TotalSize); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

func (r *ShareRepository) Get(id string) (*Share, error) {
	var sh Share
	var expiresAt sql.NullString

	row := r.storage.DB.QueryRow("SELECT id, handle, created_at, expires_at, version FROM share WHERE id = ?", id)
	if err := row.Scan(&sh.ID, &sh.Handle, &sh.CreatedAt, &expiresAt, &sh.Version); err != nil {
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

func (r *ShareRepository) GetByHandle(handle string) (*Share, error) {
	var sh Share
	var expiresAt sql.NullString

	row := r.storage.DB.QueryRow("SELECT id, handle, created_at, expires_at, version FROM share WHERE handle = ?", handle)
	if err := row.Scan(&sh.ID, &sh.Handle, &sh.CreatedAt, &expiresAt, &sh.Version); err != nil {
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

func (r *ShareRepository) Save(share *Share) (*Share, error) {
	tx, err := r.storage.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	isNew := share.ID == ""
	if isNew {
		share.ID = uuid.New().String()
		share.Version = 1
	} else {
		result, err := tx.Exec(
			"UPDATE share SET version = version + 1 WHERE id = ? AND version = ?",
			share.ID, share.Version,
		)
		if err != nil {
			return nil, err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rowsAffected != 1 {
			return nil, ErrShareVersionConflict
		}
		share.Version++
	}

	var exists bool
	err = tx.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM share WHERE handle = ? AND id != ?)",
		share.Handle, share.ID,
	).Scan(&exists)
	if err != nil {
		return nil, err
	}

	if exists {
		return nil, fmt.Errorf("%w: %s", ErrHandleExists, share.Handle)
	}

	if _, err := tx.Exec("DELETE FROM share WHERE id = ?", share.ID); err != nil {
		return nil, err
	}

	if err := r.insertShareAggregate(tx, share); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return share, nil
}

func (r *ShareRepository) insertShareAggregate(tx *sql.Tx, share *Share) error {
	var expiresAt any
	if share.ExpiresAt != nil {
		expiresAt = share.ExpiresAt.String()
	}

	if _, err := tx.Exec(
		"INSERT INTO share (id, handle, created_at, expires_at, version) VALUES (?, ?, ?, ?, ?)",
		share.ID, share.Handle, share.CreatedAt.String(), expiresAt, share.Version,
	); err != nil {
		return err
	}

	for _, file := range share.Files {
		if _, err := tx.Exec(
			"INSERT INTO file (id, name, expected_size, share_id) VALUES (?, ?, ?, ?)",
			file.ID, file.Name, file.ExpectedSize, share.ID,
		); err != nil {
			return err
		}
	}

	return nil
}

func (r *ShareRepository) Delete(shareID string) error {
	_, err := r.storage.DB.Exec("DELETE FROM share WHERE id = ?", shareID)
	return err
}

func (r *ShareRepository) getAllFilesByShareID(id string) ([]File, error) {
	var files []File
	rows, err := r.storage.DB.Query(
		"SELECT id, name, expected_size FROM file WHERE share_id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var file File
		if err := rows.Scan(&file.ID, &file.Name, &file.ExpectedSize); err != nil {
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
