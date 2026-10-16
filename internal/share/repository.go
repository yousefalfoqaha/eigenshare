package share

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"eigenshare/internal/database"
	"eigenshare/internal/timestamp"

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
	var expiresAt, trashedAt sql.NullString

	err := r.db.DB.QueryRow(query, args...).Scan(
		&sh.ID, &sh.Handle, &sh.Visibility, &sh.PasswordHash, &sh.CreatedAt, &expiresAt, &trashedAt, &sh.Version,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShareNotFound
	}

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

	files, fileRequestIDs, err := r.getAllFilesByShareID(sh.ID)
	if err != nil {
		return nil, err
	}
	sh.Files = files

	if sh.UploadRequests, err = r.getUploadRequestsByShareID(sh.ID); err != nil {
		return nil, err
	}
	fileIDsByRequest := make(map[string][]string)
	for fileID, requestID := range fileRequestIDs {
		fileIDsByRequest[requestID] = append(fileIDsByRequest[requestID], fileID)
	}
	for i := range sh.UploadRequests {
		sh.UploadRequests[i].FileIDs = fileIDsByRequest[sh.UploadRequests[i].ID]
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
		var expiresAt, trashedAt sql.NullString

		if err := rows.Scan(&sh.ID, &sh.Handle, &sh.Visibility, &sh.PasswordHash, &sh.CreatedAt, &expiresAt, &trashedAt, &sh.Version); err != nil {
			return nil, err
		}
		if sh.ExpiresAt, err = parseTimestamp(expiresAt); err != nil {
			return nil, err
		}
		if sh.TrashedAt, err = parseTimestamp(trashedAt); err != nil {
			return nil, err
		}

		shares = append(shares, &sh)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	owners, err := r.getAllOwners()
	if err != nil {
		return nil, err
	}
	files, fileRequestIDs, err := r.getAllFiles()
	if err != nil {
		return nil, err
	}
	uploadRequests, err := r.getAllUploadRequests()
	if err != nil {
		return nil, err
	}
	for _, sh := range shares {
		sh.Owners = owners[sh.ID]
		sh.Files = files[sh.ID]
		sh.UploadRequests = uploadRequests[sh.ID]

		fileIDsByRequest := make(map[string][]string)
		for fileID, requestID := range fileRequestIDs[sh.ID] {
			fileIDsByRequest[requestID] = append(fileIDsByRequest[requestID], fileID)
		}
		for i := range sh.UploadRequests {
			sh.UploadRequests[i].FileIDs = fileIDsByRequest[sh.UploadRequests[i].ID]
		}
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

// CleanupStale removes expired shares, trashed shares past the retention
// window, and expired upload requests. Runs as a single transaction so the
// batch is atomic and takes only one write-lock window in SQLite.
func (r *ShareRepository) CleanupStale() error {
	tx, err := r.db.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := timestamp.Now().String()
	if _, err := tx.Exec("DELETE FROM share WHERE expires_at IS NOT NULL AND expires_at < ?", now); err != nil {
		return fmt.Errorf("cleanup expired shares: %w", err)
	}

	threshold := timestamp.Timestamp(timestamp.Now().Time().Add(-24 * time.Hour)).String()
	if _, err := tx.Exec("DELETE FROM share WHERE trashed_at IS NOT NULL AND trashed_at < ?", threshold); err != nil {
		return fmt.Errorf("cleanup trashed shares: %w", err)
	}

	if _, err := tx.Exec("DELETE FROM upload_request WHERE expires_at IS NOT NULL AND expires_at < ?", now); err != nil {
		return fmt.Errorf("cleanup expired upload requests: %w", err)
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

// getAllFilesByShareID returns the share's files along with the raw
// file -> upload request association read from the file rows.
func (r *ShareRepository) getAllFilesByShareID(id string) ([]File, map[string]string, error) {
	rows, err := r.db.DB.Query("SELECT id, name, expected_size, upload_request_id FROM file WHERE share_id = ? ORDER BY id", id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var files []File
	fileRequestIDs := make(map[string]string)
	for rows.Next() {
		var (
			file        File
			uploadReqID sql.NullString
		)
		if err := rows.Scan(&file.ID, &file.Name, &file.ExpectedSize, &uploadReqID); err != nil {
			return nil, nil, err
		}
		if uploadReqID.Valid {
			fileRequestIDs[file.ID] = uploadReqID.String
		}
		files = append(files, file)
	}

	return files, fileRequestIDs, rows.Err()
}

func (r *ShareRepository) getAllFiles() (map[string][]File, map[string]map[string]string, error) {
	rows, err := r.db.DB.Query("SELECT share_id, id, name, expected_size, upload_request_id FROM file ORDER BY share_id, id")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	files := make(map[string][]File)
	fileRequestIDs := make(map[string]map[string]string)
	for rows.Next() {
		var (
			shareID     string
			file        File
			uploadReqID sql.NullString
		)
		if err := rows.Scan(&shareID, &file.ID, &file.Name, &file.ExpectedSize, &uploadReqID); err != nil {
			return nil, nil, err
		}
		if uploadReqID.Valid {
			if fileRequestIDs[shareID] == nil {
				fileRequestIDs[shareID] = make(map[string]string)
			}
			fileRequestIDs[shareID][file.ID] = uploadReqID.String
		}
		files[shareID] = append(files[shareID], file)
	}

	return files, fileRequestIDs, rows.Err()
}

func (r *ShareRepository) getUploadRequestsByShareID(id string) ([]UploadRequest, error) {
	rows, err := r.db.DB.Query("SELECT id, token, max_bytes, bytes_used, created_at, expires_at FROM upload_request WHERE share_id = ? ORDER BY created_at, id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploadRequests []UploadRequest
	for rows.Next() {
		ur, err := scanUploadRequest(rows.Scan)
		if err != nil {
			return nil, err
		}
		uploadRequests = append(uploadRequests, ur)
	}

	return uploadRequests, rows.Err()
}

func (r *ShareRepository) getAllUploadRequests() (map[string][]UploadRequest, error) {
	rows, err := r.db.DB.Query("SELECT share_id, id, token, max_bytes, bytes_used, created_at, expires_at FROM upload_request ORDER BY share_id, created_at, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	uploadRequests := make(map[string][]UploadRequest)
	for rows.Next() {
		var shareID string
		ur, err := scanUploadRequest(func(dest ...any) error {
			return rows.Scan(append([]any{&shareID}, dest...)...)
		})
		if err != nil {
			return nil, err
		}
		uploadRequests[shareID] = append(uploadRequests[shareID], ur)
	}

	return uploadRequests, rows.Err()
}

func scanUploadRequest(scan func(dest ...any) error) (UploadRequest, error) {
	var (
		ur        UploadRequest
		expiresAt sql.NullString
	)
	err := scan(&ur.ID, &ur.Token, &ur.MaxBytes, &ur.BytesUsed, &ur.CreatedAt, &expiresAt)
	if err != nil {
		return UploadRequest{}, err
	}
	ur.ExpiresAt, err = parseTimestamp(expiresAt)
	if err != nil {
		return UploadRequest{}, err
	}
	return ur, nil
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
		"INSERT INTO share (id, handle, visibility, password, created_at, expires_at, trashed_at, version) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		share.ID, share.Handle, string(share.Visibility), share.PasswordHash, share.CreatedAt.String(), expiresAt, trashedAt, share.Version,
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

	for _, ur := range share.UploadRequests {
		var expiresAt any
		if ur.ExpiresAt != nil {
			expiresAt = ur.ExpiresAt.String()
		}
		if _, err := tx.Exec(
			"INSERT INTO upload_request (id, token, share_id, expires_at, max_bytes, bytes_used, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			ur.ID, ur.Token, share.ID, expiresAt, ur.MaxBytes, ur.BytesUsed, ur.CreatedAt.String(),
		); err != nil {
			return err
		}
	}

	requestIDByFile := make(map[string]string)
	for _, ur := range share.UploadRequests {
		for _, fileID := range ur.FileIDs {
			requestIDByFile[fileID] = ur.ID
		}
	}

	for _, file := range share.Files {
		var uploadRequestID any
		if requestID, ok := requestIDByFile[file.ID]; ok {
			uploadRequestID = requestID
		}
		if _, err := tx.Exec(
			"INSERT INTO file (id, name, expected_size, share_id, upload_request_id) VALUES (?, ?, ?, ?, ?)",
			file.ID, file.Name, file.ExpectedSize, share.ID, uploadRequestID,
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
