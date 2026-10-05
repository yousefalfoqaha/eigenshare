package share

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"eigenshare/internal/security"
	"eigenshare/internal/timestamp"
	"eigenshare/internal/user"

	"github.com/google/uuid"
)

type ShareService struct {
	repo  *ShareRepository
	store *FileStorage
	users *user.UserService
}

func NewShareService(repo *ShareRepository, fs *FileStorage, users *user.UserService) *ShareService {
	return &ShareService{
		repo:  repo,
		store: fs,
		users: users,
	}
}

func (s *ShareService) StreamFile(shareID string, fileID string, offset int64, stream io.Reader, u *user.User) (int64, error) {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return 0, err
	}
	if !sh.CanManage(u) {
		return 0, ErrShareNotFound
	}
	if sh.Trashed() {
		return 0, ErrShareTrashed
	}

	var file *File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return 0, ErrFileNotFound
	}

	if offset >= file.ExpectedSize {
		return 0, ErrChunkOffsetExceedsSize
	}

	bytesWritten, err := s.store.WriteFileStream(shareID, file.ID, file.ExpectedSize, offset, stream)
	if err != nil {
		return 0, err
	}

	return bytesWritten, nil
}

func (s *ShareService) RequestShare(req ShareRequest, u *user.User) (*Share, error) {
	var creatorID string
	if u != nil {
		creatorID = u.ID
	}
	sh, err := NewShare("", req.Handle, req.ExpiryMinutes, req.Visibility, creatorID)
	if err != nil {
		return nil, err
	}

	sh.PasswordHash, err = hashSharePassword(req.Password)
	if err != nil {
		return nil, err
	}

	return s.repo.Save(sh)
}

func (s *ShareService) UploadFiles(shareID string, fileReqs []FileRequest, u *user.User) ([]File, error) {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return nil, err
	}
	if !sh.CanManage(u) {
		return nil, ErrShareNotFound
	}
	if sh.Trashed() {
		return nil, ErrShareTrashed
	}

	var newFiles []File

	for _, req := range fileReqs {
		file, err := NewFile(req.Name, req.ExpectedSize)
		if err != nil {
			return nil, err
		}
		file.ID = uuid.New().String()
		sh.Files = append(sh.Files, *file)
		newFiles = append(newFiles, *file)
	}

	if _, err := s.repo.Save(sh); err != nil {
		return nil, err
	}

	return newFiles, nil
}

func (s *ShareService) GetShareByHandle(handle string, u *user.User) (*Share, error) {
	sh, err := s.repo.GetByHandle(handle)
	if err != nil {
		return nil, err
	}
	if !sh.CanView(u) {
		return nil, ErrShareNotFound
	}

	return sh, nil
}

func (s *ShareService) Delete(u *user.User, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		sh, err := s.repo.Get(id)
		if err != nil {
			return err
		}
		if !sh.CanManage(u) {
			return ErrShareNotFound
		}
	}

	return s.repo.Delete(ids...)
}

func (s *ShareService) GetShares(u *user.User) ([]*Share, error) {
	shares, err := s.repo.GetAll()
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(shares, func(sh *Share) bool {
		return !sh.CanView(u)
	}), nil
}

func (s *ShareService) GetTrashedShares(u *user.User) ([]*Share, error) {
	shares, err := s.repo.GetAllTrashed()
	if err != nil {
		return nil, err
	}

	return slices.DeleteFunc(shares, func(sh *Share) bool {
		return !sh.CanManage(u)
	}), nil
}

func (s *ShareService) GetShare(id string, u *user.User) (*Share, error) {
	sh, err := s.repo.Get(id)
	if err != nil {
		return nil, err
	}
	if !sh.CanManage(u) {
		return nil, ErrShareNotFound
	}
	return sh, nil
}

func (s *ShareService) GetFileUploadState(sh *Share, u *user.User, fileID string) (FileUploadState, error) {
	if !sh.CanView(u) {
		return FileUploadState{}, ErrShareNotFound
	}

	return s.store.FileUploadState(sh.ID, fileID)
}

func (s *ShareService) GetShareUploadState(sh *Share, u *user.User) (map[string]FileUploadState, error) {
	if !sh.CanView(u) {
		return nil, ErrShareNotFound
	}

	states := make(map[string]FileUploadState, len(sh.Files))
	for _, f := range sh.Files {
		st, err := s.store.FileUploadState(sh.ID, f.ID)
		if err != nil {
			return nil, err
		}
		states[f.ID] = st
	}

	return states, nil
}

// ListSummaries returns summaries for every share the user can view.
func (s *ShareService) ListSummaries(u *user.User) ([]ShareSummary, error) {
	shares, err := s.GetShares(u)
	if err != nil {
		return nil, err
	}

	summaries := make([]ShareSummary, 0, len(shares))
	for _, sh := range shares {
		summary, err := s.summarize(sh, u)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// ListTrashedSummaries returns summaries for every trashed share the user can manage.
func (s *ShareService) ListTrashedSummaries(u *user.User) ([]ShareSummary, error) {
	shares, err := s.GetTrashedShares(u)
	if err != nil {
		return nil, err
	}

	summaries := make([]ShareSummary, 0, len(shares))
	for _, sh := range shares {
		summary, err := s.summarize(sh, u)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// GetSummary returns the summary for a single share.
func (s *ShareService) GetSummary(id string, u *user.User) (*ShareSummary, error) {
	sh, err := s.GetShare(id, u)
	if err != nil {
		return nil, err
	}

	summary, err := s.summarize(sh, u)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (s *ShareService) summarize(sh *Share, u *user.User) (ShareSummary, error) {
	states, err := s.GetShareUploadState(sh, u)
	if err != nil {
		return ShareSummary{}, err
	}

	return ShareSummary{
		Share:      sh,
		Upload:     states,
		OwnerNames: s.ownerNames(sh),
	}, nil
}

func (s *ShareService) ownerNames(sh *Share) []string {
	users, _ := s.users.GetAllUsers()
	names := make([]string, 0, len(sh.Owners))
	for _, u := range users {
		if sh.IsOwner(u) {
			names = append(names, u.Username)
		}
	}
	return names
}

func (s *ShareService) DownloadAll(shareID string, w io.Writer) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}

	return s.store.CreateShareZipStream(*sh, w)
}

func (s *ShareService) DownloadFile(shareID string, fileID string) (*File, string, error) {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return nil, "", err
	}

	var file *File
	for i, f := range sh.Files {
		if f.ID == fileID {
			file = &sh.Files[i]
			break
		}
	}
	if file == nil {
		return nil, "", ErrFileNotFound
	}

	state, err := s.store.FileUploadState(sh.ID, file.ID)
	if err != nil {
		return nil, "", err
	}

	if state.Written < file.ExpectedSize {
		return nil, "", ErrFileNotComplete
	}

	path, err := s.store.ShareFilePath(sh.ID, file.ID)
	if err != nil {
		return nil, "", err
	}

	return file, path, nil
}

func (s *ShareService) DeleteFile(shareID, fileID string, u *user.User) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	var removedFileID string
	for i, f := range sh.Files {
		if f.ID == fileID {
			removedFileID = f.ID
			sh.Files = append(sh.Files[:i], sh.Files[i+1:]...)
			break
		}
	}
	if removedFileID == "" {
		return ErrFileNotFound
	}

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) TrashShare(shareID string, u *user.User) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	now := timestamp.Now()
	sh.TrashedAt = &now

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) RecoverShare(shareID string, u *user.User) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if !sh.Trashed() {
		return ErrShareNotTrashed
	}

	sh.TrashedAt = nil

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) AddOwner(shareID string, u *user.User, newOwnerID string) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	if _, err := s.users.GetUser(newOwnerID); err != nil {
		return err
	}

	if slices.Contains(sh.Owners, newOwnerID) {
		return ErrOwnerExists
	}

	sh.Owners = append(sh.Owners, newOwnerID)

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) RemoveOwner(shareID string, u *user.User, ownerID string) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	idx := slices.Index(sh.Owners, ownerID)
	if idx == -1 {
		return ErrShareNotFound
	}
	if len(sh.Owners) == 1 {
		return ErrLastOwner
	}

	sh.Owners = slices.Delete(sh.Owners, idx, idx+1)

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) SetVisibility(shareID string, u *user.User, visibility Visibility) error {
	if !visibility.Valid() {
		return fmt.Errorf("visibility must be public or private: %w", ErrInvalidVisibility)
	}

	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	if sh.Visibility == visibility {
		return nil
	}

	sh.Visibility = visibility

	_, err = s.repo.Save(sh)

	return err
}

func (s *ShareService) SetPassword(shareID string, u *user.User, raw string, remove bool) error {
	sh, err := s.repo.Get(shareID)
	if err != nil {
		return err
	}
	if !sh.CanManage(u) {
		return ErrShareNotFound
	}
	if sh.Trashed() {
		return ErrShareTrashed
	}

	switch {
	case remove:
		sh.PasswordHash = ""
	case strings.TrimSpace(raw) != "":
		hash, err := hashSharePassword(raw)
		if err != nil {
			return err
		}
		sh.PasswordHash = hash
	}

	_, err = s.repo.Save(sh)

	return err
}

// hashSharePassword normalizes a raw password and returns a bcrypt hash, or
// "" when no password should be set. Rules:
//   - "" or whitespace-only => no password
//   - otherwise the value is used exactly as typed (no trimming)
//   - non-empty passwords must be 3-72 bytes
func hashSharePassword(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if len(raw) < 3 {
		return "", fmt.Errorf("password must be at least 3 characters: %w", security.ErrInvalidPassword)
	}
	if len(raw) > 72 {
		return "", fmt.Errorf("password must be at most 72 characters: %w", security.ErrInvalidPassword)
	}
	return security.HashPassword(raw)
}
