package share

import (
	"database/sql"
	"fmt"

	"eigenshare/backend/file"
	"eigenshare/backend/storage"
)

type Repository struct {
	storage        *storage.Storage
	fileRepository *file.Repository
}

func NewRepository(s *storage.Storage) *Repository {
	return &Repository{storage: s}
}

func (r *Repository) GetAll() ([]Share, error) {
	var shares []Share

	rows, err := r.storage.DB.Query("SELECT id FROM share")
	if err != nil {
		return nil, fmt.Errorf("Failed to query database for all files.")
	}

	for rows.Next() {
		var s Share
		if err := rows.Scan(&s.ID); err != nil {
			return nil, fmt.Errorf("failed to read rows.")
		}

		fileIds, err := r.fileRepository.GetAllByShareID(s.ID)
		if err != nil {
			return nil, err
		}

		s.Files = fileIds
		shares = append(shares, s)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	return shares, nil
}

func (r *Repository) Get(id string) (*Share, error) {
	var share Share

	row := r.storage.DB.QueryRow("SELECT id FROM share WHERE id = ?", id)
	if err := row.Scan(&share.ID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("Get share by ID %d: no such share", id)
		}

		return nil, fmt.Errorf("sharesById %d: %v", id, err)
	}
	if err := row.Err(); err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	fileIds, err := r.fileRepository.GetAllByShareID(share.ID)
	if err != nil {
		return nil, err
	}

	share.Files = fileIds

	return &share, nil
}
