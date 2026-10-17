package share

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"eigenshare/internal/database"

	"github.com/google/uuid"
)

type ShareRepository struct {
	db *database.Database
}

func NewShareRepository(db *database.Database) *ShareRepository {
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
	query := "SELECT id, handle, visibility, password, created_at, expires_at, trashed_at, version FROM share " + whereClause

	var sh Share
	var expiresAt, trashedAt sql.NullTime

	err := r.db.DB.QueryRow(query, args...).Scan(
		&sh.ID, &sh.Handle, &sh.Visibility, &sh.PasswordHash, &sh.CreatedAt, &expiresAt, &trashedAt, &sh.Version,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShareNotFound
	}

	if err != nil {
		return nil, err
	}

	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		sh.ExpiresAt = &t
	}
	if trashedAt.Valid {
		t := trashedAt.Time.UTC()
		sh.TrashedAt = &t
	}
	sh.CreatedAt = sh.CreatedAt.UTC()

	if sh.Owners, err = r.getOwnersByShareID(sh.ID); err != nil {
		return nil, err
	}

	files, err := r.getAllFilesByShareID(sh.ID)
	if err != nil {
		return nil, err
	}
	sh.Files = files

	if sh.UploadLinks, err = r.getUploadLinksByShareID(sh.ID); err != nil {
		return nil, err
	}

	return &sh, nil
}

func (r *ShareRepository) GetAll() ([]*Share, error) {
	return r.fetchAllShares("WHERE trashed_at IS NULL")
}

func (r *ShareRepository) GetAllTrashed() ([]*Share, error) {
	return r.fetchAllShares("WHERE trashed_at IS NOT NULL")
}

func (r *ShareRepository) fetchAllShares(whereClause string, args ...any) ([]*Share, error) {
	query := "SELECT id, handle, visibility, password, created_at, expires_at, trashed_at, version FROM share " + whereClause

	rows, err := r.db.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []*Share
	for rows.Next() {
		var sh Share
		var expiresAt, trashedAt sql.NullTime

		if err := rows.Scan(&sh.ID, &sh.Handle, &sh.Visibility, &sh.PasswordHash, &sh.CreatedAt, &expiresAt, &trashedAt, &sh.Version); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			t := expiresAt.Time.UTC()
			sh.ExpiresAt = &t
		}
		if trashedAt.Valid {
			t := trashedAt.Time.UTC()
			sh.TrashedAt = &t
		}
		sh.CreatedAt = sh.CreatedAt.UTC()

		shares = append(shares, &sh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	owners, err := r.getAllOwners()
	if err != nil {
		return nil, err
	}
	files, err := r.getAllFiles()
	if err != nil {
		return nil, err
	}
	uploadLinks, err := r.getAllUploadLinks()
	if err != nil {
		return nil, err
	}
	for _, sh := range shares {
		sh.Owners = owners[sh.ID]
		sh.Files = files[sh.ID]
		sh.UploadLinks = uploadLinks[sh.ID]
	}

	return shares, nil
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

func (r *ShareRepository) CleanupStale() error {
	tx, err := r.db.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if _, err := tx.Exec("DELETE FROM share WHERE expires_at IS NOT NULL AND expires_at < ?", now); err != nil {
		return fmt.Errorf("cleanup expired shares: %w", err)
	}

	threshold := now.Add(-24 * time.Hour)
	if _, err := tx.Exec("DELETE FROM share WHERE trashed_at IS NOT NULL AND trashed_at < ?", threshold); err != nil {
		return fmt.Errorf("cleanup trashed shares: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM upload_link WHERE expires_at IS NOT NULL AND expires_at < ?", now); err != nil {
		return fmt.Errorf("cleanup expired upload links: %w", err)
	}

	return tx.Commit()
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
	rows, err := r.db.DB.Query("SELECT id, name, expected_size FROM file WHERE share_id = ? ORDER BY id", id)
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

func (r *ShareRepository) getAllFiles() (map[string][]File, error) {
	rows, err := r.db.DB.Query("SELECT share_id, id, name, expected_size FROM file ORDER BY share_id, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make(map[string][]File)
	for rows.Next() {
		var (
			shareID string
			file    File
		)
		if err := rows.Scan(&shareID, &file.ID, &file.Name, &file.ExpectedSize); err != nil {
			return nil, err
		}
		files[shareID] = append(files[shareID], file)
	}

	return files, rows.Err()
}

func (r *ShareRepository) getUploadLinksByShareID(id string) ([]UploadLink, error) {
	rows, err := r.db.DB.Query("SELECT id, token, max_bytes, bytes_used, created_at, expires_at FROM upload_link WHERE share_id = ? ORDER BY created_at, id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploadLinks []UploadLink
	for rows.Next() {
		ul, err := scanUploadLink(rows.Scan)
		if err != nil {
			return nil, err
		}
		uploadLinks = append(uploadLinks, ul)
	}

	return uploadLinks, rows.Err()
}

func (r *ShareRepository) getAllUploadLinks() (map[string][]UploadLink, error) {
	rows, err := r.db.DB.Query("SELECT share_id, id, token, max_bytes, bytes_used, created_at, expires_at FROM upload_link ORDER BY share_id, created_at, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	uploadLinks := make(map[string][]UploadLink)
	for rows.Next() {
		var shareID string
		ul, err := scanUploadLink(func(dest ...any) error {
			return rows.Scan(append([]any{&shareID}, dest...)...)
		})
		if err != nil {
			return nil, err
		}
		uploadLinks[shareID] = append(uploadLinks[shareID], ul)
	}

	return uploadLinks, rows.Err()
}

func scanUploadLink(scan func(dest ...any) error) (UploadLink, error) {
	var (
		ul        UploadLink
		expiresAt sql.NullTime
	)
	err := scan(&ul.ID, &ul.Token, &ul.MaxBytes, &ul.BytesUsed, &ul.CreatedAt, &expiresAt)
	if err != nil {
		return UploadLink{}, err
	}
	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		ul.ExpiresAt = &t
	}
	ul.CreatedAt = ul.CreatedAt.UTC()
	return ul, nil
}

func insertShareAggregate(tx *sql.Tx, share *Share) error {
	var expiresAt any
	if share.ExpiresAt != nil {
		expiresAt = share.ExpiresAt.UTC()
	}
	var trashedAt any
	if share.TrashedAt != nil {
		trashedAt = share.TrashedAt.UTC()
	}

	if _, err := tx.Exec(
		"INSERT INTO share (id, handle, visibility, password, created_at, expires_at, trashed_at, version) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		share.ID, share.Handle, string(share.Visibility), share.PasswordHash, share.CreatedAt.UTC(), expiresAt, trashedAt, share.Version,
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

	for _, ul := range share.UploadLinks {
		var expiresAt any
		if ul.ExpiresAt != nil {
			expiresAt = ul.ExpiresAt.UTC()
		}
		if _, err := tx.Exec(
			"INSERT INTO upload_link (id, token, share_id, expires_at, max_bytes, bytes_used, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			ul.ID, ul.Token, share.ID, expiresAt, ul.MaxBytes, ul.BytesUsed, ul.CreatedAt.UTC(),
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
