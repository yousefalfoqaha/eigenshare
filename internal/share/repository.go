package share

import (
	"database/sql"
	"fmt"
	"strings"

	"eigenshare/internal/infra"
	"eigenshare/internal/timestamp"

	"github.com/google/uuid"
)

type ShareRepository struct {
	db *infra.Database
}

func NewShareRepository(db *infra.Database) *ShareRepository {
	return &ShareRepository{db: db}
}

func (r *ShareRepository) Get(id string) (*Share, error) {
	return r.fetchShareAggregate("WHERE id = ?", id)
}

func (r *ShareRepository) GetByHandle(handle string) (*Share, error) {
	return r.fetchShareAggregate("WHERE handle = ? AND trashed_at IS NULL", handle)
}

func (r *ShareRepository) Exists(id string) (bool, error) {
	var exists bool
	err := r.db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM share WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

func (r *ShareRepository) fetchShareAggregate(whereClause string, args ...any) (*Share, error) {
	query := "SELECT id, handle, visibility, created_at, expires_at, trashed_at, version FROM share " + whereClause

	var sh Share
	var expiresAt, trashedAt sql.NullString

	err := r.db.DB.QueryRow(query, args...).Scan(
		&sh.ID, &sh.Handle, &sh.Visibility, &sh.CreatedAt, &expiresAt, &trashedAt, &sh.Version,
	)
	if err != nil {
		return nil, err
	}

	if sh.ExpiresAt, err = parseTimestamp(expiresAt); err != nil {
		return nil, err
	}
	if sh.TrashedAt, err = parseTimestamp(trashedAt); err != nil {
		return nil, err
	}

	if sh.Owners, err = r.getOwnersByShareID(sh.ID); err != nil {
		return nil, err
	}

	if sh.Files, err = r.getAllFilesByShareID(sh.ID); err != nil {
		return nil, err
	}

	return &sh, nil
}

func (r *ShareRepository) GetAll() ([]ShareSummary, error) {
	return r.fetchShareSummaries("WHERE s.trashed_at IS NULL")
}

func (r *ShareRepository) GetAllTrashed() ([]ShareSummary, error) {
	return r.fetchShareSummaries("WHERE s.trashed_at IS NOT NULL")
}

func (r *ShareRepository) fetchShareSummaries(whereClause string, args ...any) ([]ShareSummary, error) {
	query := `
		SELECT
			s.id,
			s.handle,
			s.visibility,
			s.expires_at,
			s.trashed_at,
			COUNT(f.id) AS totalFiles,
			COALESCE(SUM(f.expected_size), 0) AS totalSize
		FROM share s
		LEFT JOIN file f ON s.id = f.share_id
		` + whereClause + `
		GROUP BY s.id, s.handle, s.visibility`

	rows, err := r.db.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []ShareSummary
	for rows.Next() {
		var s ShareSummary
		if err := rows.Scan(&s.ID, &s.Handle, &s.Visibility, &s.ExpiresAt, &s.TrashedAt, &s.TotalFiles, &s.TotalSize); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	owners, err := r.getAllOwners()
	if err != nil {
		return nil, err
	}
	for i := range summaries {
		summaries[i].Owners = owners[summaries[i].ID]
	}

	return summaries, nil
}

func (r *ShareRepository) Save(share *Share) (*Share, error) {
	tx, err := r.db.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if share.ID == "" {
		share.ID = uuid.New().String()
		share.Version = 1
	} else {
		result, err := tx.Exec("UPDATE share SET version = version + 1 WHERE id = ? AND version = ?", share.ID, share.Version)
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
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM share WHERE handle = ? AND id != ?)", share.Handle, share.ID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("%w: %s", ErrHandleExists, share.Handle)
	}

	if _, err := tx.Exec("DELETE FROM share WHERE id = ?", share.ID); err != nil {
		return nil, err
	}
	if err := insertShareAggregate(tx, share); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return share, nil
}

func (r *ShareRepository) Delete(ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?, ", len(ids)), ", ")
	query := fmt.Sprintf("DELETE FROM share WHERE id IN (%s)", placeholders)

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	_, err := r.db.DB.Exec(query, args...)
	return err
}

func (r *ShareRepository) GetKeys() (map[string]struct{}, error) {
	keys := make(map[string]struct{})

	rows, err := r.db.DB.Query("SELECT id FROM share")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		keys[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	fRows, err := r.db.DB.Query("SELECT share_id, id FROM file")
	if err != nil {
		return nil, err
	}
	defer fRows.Close()

	for fRows.Next() {
		var shareID, fileID string
		if err := fRows.Scan(&shareID, &fileID); err != nil {
			return nil, err
		}
		keys[shareID+"/"+fileID] = struct{}{}
	}
	if err := fRows.Err(); err != nil {
		return nil, err
	}

	return keys, nil
}

func (r *ShareRepository) getOwnersByShareID(id string) ([]string, error) {
	rows, err := r.db.DB.Query("SELECT user_id FROM share_owner WHERE share_id = ? ORDER BY user_id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var owners []string
	for rows.Next() {
		var ownerID string
		if err := rows.Scan(&ownerID); err != nil {
			return nil, err
		}
		owners = append(owners, ownerID)
	}

	return owners, rows.Err()
}

func (r *ShareRepository) getAllOwners() (map[string][]string, error) {
	rows, err := r.db.DB.Query("SELECT share_id, user_id FROM share_owner")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	owners := make(map[string][]string)
	for rows.Next() {
		var shareID, userID string
		if err := rows.Scan(&shareID, &userID); err != nil {
			return nil, err
		}
		owners[shareID] = append(owners[shareID], userID)
	}

	return owners, rows.Err()
}

func (r *ShareRepository) getAllFilesByShareID(id string) ([]File, error) {
	rows, err := r.db.DB.Query("SELECT id, name, expected_size FROM file WHERE share_id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []File
	for rows.Next() {
		var file File
		if err := rows.Scan(&file.ID, &file.Name, &file.ExpectedSize); err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

func insertShareAggregate(tx *sql.Tx, share *Share) error {
	var expiresAt any
	if share.ExpiresAt != nil {
		expiresAt = share.ExpiresAt.String()
	}
	var trashedAt any
	if share.TrashedAt != nil {
		trashedAt = share.TrashedAt.String()
	}

	if _, err := tx.Exec(
		"INSERT INTO share (id, handle, visibility, created_at, expires_at, trashed_at, version) VALUES (?, ?, ?, ?, ?, ?, ?)",
		share.ID, share.Handle, string(share.Visibility), share.CreatedAt.String(), expiresAt, trashedAt, share.Version,
	); err != nil {
		return err
	}

	for _, ownerID := range share.Owners {
		if _, err := tx.Exec(
			"INSERT INTO share_owner (share_id, user_id) VALUES (?, ?)",
			share.ID, ownerID,
		); err != nil {
			return err
		}
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

func parseTimestamp(ns sql.NullString) (*timestamp.Timestamp, error) {
	if !ns.Valid {
		return nil, nil
	}
	var t timestamp.Timestamp
	if err := t.Scan(ns.String); err != nil {
		return nil, err
	}
	return &t, nil
}
