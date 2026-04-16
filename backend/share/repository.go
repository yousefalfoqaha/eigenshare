package share

import (
	"database/sql"
	"eigenshare/backend/storage"
)

type Repository struct {
	storage *storage.Storage
}

func NewRepository(s *storage.Storage) *Repository {
	return &Repository{storage: s}
}

func (r *Repository) GetAll() ([]ShareSummary, error) {
	var summaries []ShareSummary

	query := `
		SELECT 
			s.id, 
			COUNT(f.id) AS totalFiles, 
			COALESCE(SUM(f.size), 0) AS totalSize
		FROM share s
		LEFT JOIN file f ON s.id = f.share_id
		GROUP BY s.id`

	rows, err := r.storage.DB.Query(query)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var s ShareSummary
		if err := rows.Scan(&s.ID, &s.TotalFiles, &s.TotalSize); err != nil {
			return nil, err
		}

		summaries = append(summaries, s)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
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

	row := r.storage.DB.QueryRow("SELECT id, handle FROM share WHERE id = ?", id)
	if err := row.Scan(&share.ID, &share.Handle); err != nil {
		return nil, err
	}
	if err := row.Err(); err != nil {
		return nil, err
	}

	files, err := r.getAllFilesByShareID(share.ID)
	if err != nil {
		return nil, err
	}

	share.Files = files

	return &share, nil
}

func (r *Repository) makeTransaction(o func(tx *sql.Tx) error) error {
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

func (r *Repository) Save(s Share) (*Share, error) {
	err := r.makeTransaction(func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO share (id) VALUES (?)", s.ID)
		if err != nil {
			return err
		}

		for _, f := range s.Files {
			_, err := tx.Exec(
				"INSERT INTO file (id, name, size, share_id) VALUES (?, ?, ?, ?)",
				f.ID, f.Name, f.Size, s.ID,
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &s, nil
}

func (r *Repository) getAllFilesByShareID(id string) ([]File, error) {
	var files []File
	rows, err := r.storage.DB.Query("SELECT id, name, size, bitmask FROM File WHERE share_id = ?", id)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var file File
		if err := rows.Scan(&file.ID, &file.Name, &file.Size, &file.Bitmask); err != nil {
			return nil, err
		}

		files = append(files, file)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return files, nil
}
