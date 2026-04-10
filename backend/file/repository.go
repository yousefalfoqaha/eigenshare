package file

import (
	"fmt"

	"eigenshare/backend/storage"
)

type Repository struct {
	storage *storage.Storage
}

func NewRepository(s *storage.Storage) *Repository {
	return &Repository{storage: s}
}

func (r *Repository) GetAllByShareID(id string) ([]string, error) {
	var fileIds []string

	rows, err := r.storage.DB.Query("SELECT id FROM File WHERE share = ?", id)
	if err != nil {
		return nil, fmt.Errorf("failed to query database for all files.")
	}

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to read rows.")
		}

		fileIds = append(fileIds, id)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	return fileIds, nil
}
