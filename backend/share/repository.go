package share

import (
	"eigenshare/backend/storage"
)

type Repository struct {
	storage *storage.Storage
}

func NewRepository(s *storage.Storage) *Repository {
	return &Repository{storage: s}
}

func (r *Repository) GetAll() ([]Share, error) {
	var shares []Share

	rows, err := r.storage.DB.Query("SELECT id FROM share")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var s Share
		if err := rows.Scan(&s.ID); err != nil {
			return nil, err
		}

		fileIds, err := r.GetAllFilesByShareID(s.ID)
		if err != nil {
			return nil, err
		}

		s.Files = fileIds
		shares = append(shares, s)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return shares, nil
}

func (r *Repository) Exists(id string) (bool, error) {
	var exists bool

	err := r.storage.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM share WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (r *Repository) Get(id string) (*Share, error) {
	var share Share

	row := r.storage.DB.QueryRow("SELECT id FROM share WHERE id = ?", id)
	if err := row.Scan(&share.ID); err != nil {
		return nil, err
	}
	if err := row.Err(); err != nil {
		return nil, err
	}

	files, err := r.GetAllFilesByShareID(share.ID)
	if err != nil {
		return nil, err
	}

	share.Files = files

	return &share, nil
}

func (r *Repository) Save(s Share) error {
	_, err := r.storage.DB.Exec("INSERT INTO share (id) VALUES (?)", s.ID)
	if err != nil {
		return err
	}

	for _, f := range s.Files {
		_, err := r.storage.DB.Exec(
			"INSERT INTO file (id, name, size, share_id) VALUES (?, ?, ?, ?)",
			f.ID, f.Name, f.Size, s.ID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) GetAllFilesByShareID(id string) ([]File, error) {
	var files []File
	rows, err := r.storage.DB.Query("SELECT id, name, size FROM File WHERE share_id = ?", id)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var file File
		if err := rows.Scan(&file.ID, &file.Name, &file.Size); err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return files, nil
}
