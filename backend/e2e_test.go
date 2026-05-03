package main_test

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"eigenshare/backend/api"
	"eigenshare/backend/handler"
	"eigenshare/backend/middleware"
	"eigenshare/backend/repository"
	"eigenshare/backend/service"
	"eigenshare/backend/share"
	"eigenshare/backend/storage"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type testServer struct {
	URL          string
	db           *sql.DB
	shareService *service.ShareService
	srv          *httptest.Server
}

func setupTestServer(t *testing.T) *testServer {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dataPath := filepath.Join(tmpDir, "data")

	dbStorage := storage.NewDatabaseStorage(dbPath)
	fileStorage := storage.NewFileStorage(dataPath)
	shareRepo := repository.NewShareRepository(dbStorage)
	shareSvc := service.NewShareService(shareRepo, fileStorage)
	shareCtrl := handler.NewShareController(shareSvc)

	router := http.NewServeMux()
	shareCtrl.RegisterHandlers(router)

	srv := httptest.NewServer(middleware.Logging(router))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { dbStorage.DB.Close() })

	return &testServer{
		URL:          srv.URL,
		db:           dbStorage.DB,
		shareService: shareSvc,
		srv:          srv,
	}
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "resources", "e2e_tests", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func doRequest(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func createShare(t *testing.T, srv *testServer, body handler.ShareRequest) share.Share {
	t.Helper()
	resp := doRequest(t, http.MethodPost, srv.URL+"/shares", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errResp api.Error
		decodeJSON(t, resp, &errResp)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, errResp.Message)
	}

	var sh share.Share
	decodeJSON(t, resp, &sh)

	return sh
}

func streamFile(t *testing.T, srv *testServer, shareID, fileID string, data []byte, offset int64) *http.Response {
	t.Helper()
	url := fmt.Sprintf("%s/shares/%s/files/%s/stream", srv.URL, shareID, fileID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Write-Offset", strconv.FormatInt(offset, 10))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	return resp
}

func getShare(t *testing.T, srv *testServer, id string) (*share.Share, int) {
	t.Helper()
	resp := doRequest(t, http.MethodGet, srv.URL+"/shares/"+id, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode
	}

	var sh share.Share
	decodeJSON(t, resp, &sh)

	return &sh, resp.StatusCode
}

func getShares(t *testing.T, srv *testServer) []share.ShareSummary {
	t.Helper()
	resp := doRequest(t, http.MethodGet, srv.URL+"/shares", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var shares []share.ShareSummary
	decodeJSON(t, resp, &shares)
	return shares
}

func deleteShare(t *testing.T, srv *testServer, id string) int {
	t.Helper()
	resp := doRequest(t, http.MethodDelete, srv.URL+"/shares/"+id, nil)
	defer resp.Body.Close()

	return resp.StatusCode
}

func getProgress(t *testing.T, srv *testServer, id string) int64 {
	t.Helper()
	resp := doRequest(t, http.MethodGet, srv.URL+"/shares/"+id+"/progress", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]int64
	decodeJSON(t, resp, &result)

	return result["bytesWritten"]
}

func defaultShareBody(handle string, expiryMinutes int) handler.ShareRequest {
	return handler.ShareRequest{
		Handle:        handle,
		ExpiryMinutes: expiryMinutes,
		Files: []handler.FileRequest{
			{Name: "test.txt", ExpectedSize: 100, Order: 0},
		},
	}
}

func extractZipFiles(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	files := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = content
	}

	return files
}

func TestCreateShareWithExpiry(t *testing.T) {
	srv := setupTestServer(t)
	sh := createShare(t, srv, defaultShareBody("expiry-test", 60))

	if sh.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if sh.Handle != "expiry-test" {
		t.Fatalf("expected handle 'expiry-test', got %q", sh.Handle)
	}
	if sh.ExpiresAt == nil {
		t.Fatal("expected expiresAt to be set")
	}
	if sh.CreatedAt.IsZero() {
		t.Fatal("expected createdAt to be set")
	}
	if len(sh.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(sh.Files))
	}
	if sh.Files[0].Name != "test.txt" {
		t.Fatalf("expected file name 'test.txt', got %q", sh.Files[0].Name)
	}
}

func TestCreatePermanentShare(t *testing.T) {
	srv := setupTestServer(t)
	sh := createShare(t, srv, defaultShareBody("permanent-test", 0))

	if sh.ExpiresAt != nil {
		t.Fatalf("expected expiresAt to be null, got %v", *sh.ExpiresAt)
	}
}

func TestCreateShareInvalidExpiry(t *testing.T) {
	srv := setupTestServer(t)
	resp := doRequest(t, http.MethodPost, srv.URL+"/shares", defaultShareBody("bad-expiry", -1))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateShareEmptyHandle(t *testing.T) {
	srv := setupTestServer(t)
	body := defaultShareBody("", 60)
	resp := doRequest(t, http.MethodPost, srv.URL+"/shares", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateShareDuplicateHandle(t *testing.T) {
	srv := setupTestServer(t)
	createShare(t, srv, defaultShareBody("dup-handle", 60))

	resp := doRequest(t, http.MethodPost, srv.URL+"/shares", defaultShareBody("dup-handle", 60))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestStreamFile(t *testing.T) {
	srv := setupTestServer(t)
	sh := createShare(t, srv, defaultShareBody("stream-test", 60))

	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}

	resp := streamFile(t, srv, sh.ID, sh.Files[0].ID, data, 0)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestStreamFileInvalidOffset(t *testing.T) {
	srv := setupTestServer(t)
	sh := createShare(t, srv, defaultShareBody("offset-test", 60))

	data := make([]byte, 10)
	resp := streamFile(t, srv, sh.ID, sh.Files[0].ID, data, 200)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestStreamFileNonExistentShare(t *testing.T) {
	srv := setupTestServer(t)

	data := make([]byte, 10)
	resp := streamFile(t, srv, "nonexistent-id", "fake-file-id", data, 0)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetShare(t *testing.T) {
	srv := setupTestServer(t)
	created := createShare(t, srv, defaultShareBody("get-test", 60))

	got, status := getShare(t, srv, created.ID)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if got.ID != created.ID {
		t.Fatalf("expected ID %q, got %q", created.ID, got.ID)
	}
	if got.Handle != "get-test" {
		t.Fatalf("expected handle 'get-test', got %q", got.Handle)
	}
	if len(got.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(got.Files))
	}
}

func TestGetShares(t *testing.T) {
	srv := setupTestServer(t)
	createShare(t, srv, defaultShareBody("list-test-1", 60))
	createShare(t, srv, defaultShareBody("list-test-2", 60))

	shares := getShares(t, srv)
	if len(shares) < 2 {
		t.Fatalf("expected at least 2 shares, got %d", len(shares))
	}
}

func TestGetShareNotFound(t *testing.T) {
	srv := setupTestServer(t)

	_, status := getShare(t, srv, "nonexistent-id")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestGetProgress(t *testing.T) {
	srv := setupTestServer(t)
	sh := createShare(t, srv, defaultShareBody("progress-test", 60))

	data := make([]byte, 50)
	for i := range data {
		data[i] = byte(i)
	}

	resp := streamFile(t, srv, sh.ID, sh.Files[0].ID, data, 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	written := getProgress(t, srv, sh.ID)
	if written != 50 {
		t.Fatalf("expected 50 bytes written, got %d", written)
	}
}

func TestDeleteShare(t *testing.T) {
	srv := setupTestServer(t)
	sh := createShare(t, srv, defaultShareBody("delete-test", 60))

	status := deleteShare(t, srv, sh.ID)
	if status != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", status)
	}

	_, getStatus := getShare(t, srv, sh.ID)
	if getStatus != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getStatus)
	}
}

func TestDeleteNonExistentShare(t *testing.T) {
	srv := setupTestServer(t)

	status := deleteShare(t, srv, "nonexistent-id")
	if status != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", status)
	}
}

func TestExpiryCleanup(t *testing.T) {
	srv := setupTestServer(t)
	sh := createShare(t, srv, defaultShareBody("expiring", 60))

	past := time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339Nano)
	_, err := srv.db.Exec("UPDATE share SET expires_at = ? WHERE id = ?", past, sh.ID)
	if err != nil {
		t.Fatal(err)
	}

	ids, err := srv.shareService.GetExpiredShares()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("expected expired shares, got none")
	}

	err = srv.shareService.Delete(ids...)
	if err != nil {
		t.Fatal(err)
	}

	_, status := getShare(t, srv, sh.ID)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 after expiry cleanup, got %d", status)
	}
}

func TestPermanentShareNotExpired(t *testing.T) {
	srv := setupTestServer(t)
	createShare(t, srv, defaultShareBody("permanent", 0))

	ids, err := srv.shareService.GetExpiredShares()
	if err != nil {
		t.Fatal(err)
	}

	if len(ids) != 0 {
		t.Fatalf("expected no expired shares, got %d", len(ids))
	}
}

func TestDeleteMultipleShares(t *testing.T) {
	srv := setupTestServer(t)
	sh1 := createShare(t, srv, defaultShareBody("batch-1", 0))
	sh2 := createShare(t, srv, defaultShareBody("batch-2", 0))

	err := srv.shareService.Delete(sh1.ID, sh2.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, status1 := getShare(t, srv, sh1.ID)
	if status1 != http.StatusNotFound {
		t.Fatalf("expected 404 for first share, got %d", status1)
	}

	_, status2 := getShare(t, srv, sh2.ID)
	if status2 != http.StatusNotFound {
		t.Fatalf("expected 404 for second share, got %d", status2)
	}
}

func TestDownloadCompleteShare(t *testing.T) {
	srv := setupTestServer(t)

	helloData := readFile(t, "hello.txt")
	binaryData := readFile(t, "binary.bin")

	body := handler.ShareRequest{
		Handle:        "download-test",
		ExpiryMinutes: 60,
		Files: []handler.FileRequest{
			{Name: "hello.txt", ExpectedSize: int64(len(helloData)), Order: 0},
			{Name: "binary.bin", ExpectedSize: int64(len(binaryData)), Order: 1},
		},
	}
	sh := createShare(t, srv, body)

	resp := streamFile(t, srv, sh.ID, sh.Files[0].ID, helloData, 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for hello.txt, got %d", resp.StatusCode)
	}

	resp = streamFile(t, srv, sh.ID, sh.Files[1].ID, binaryData, 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for binary.bin, got %d", resp.StatusCode)
	}

	dlResp, err := http.Get(srv.URL + "/shares/" + sh.ID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", dlResp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(dlResp.Body)
	if err != nil {
		t.Fatal(err)
	}

	files := extractZipFiles(t, bodyBytes)
	if len(files) != 2 {
		t.Fatalf("expected 2 files in zip, got %d", len(files))
	}

	if !bytes.Equal(files["hello.txt"], helloData) {
		t.Fatalf("hello.txt content mismatch: expected %d bytes, got %d bytes", len(helloData), len(files["hello.txt"]))
	}
	if !bytes.Equal(files["binary.bin"], binaryData) {
		t.Fatalf("binary.bin content mismatch: expected %d bytes, got %d bytes", len(binaryData), len(files["binary.bin"]))
	}
}

func TestStreamWithResume(t *testing.T) {
	srv := setupTestServer(t)

	largeData := readFile(t, "sample.pdf")
	midpoint := int64(len(largeData) / 2)

	body := handler.ShareRequest{
		Handle:        "resume-test",
		ExpiryMinutes: 60,
		Files: []handler.FileRequest{
			{Name: "sample.pdf", ExpectedSize: int64(len(largeData)), Order: 0},
		},
	}
	sh := createShare(t, srv, body)

	resp := streamFile(t, srv, sh.ID, sh.Files[0].ID, largeData[:midpoint], 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for first chunk, got %d", resp.StatusCode)
	}

	written := getProgress(t, srv, sh.ID)
	if written != midpoint {
		t.Fatalf("expected %d bytes written after first chunk, got %d", midpoint, written)
	}

	resp = streamFile(t, srv, sh.ID, sh.Files[0].ID, largeData[midpoint:], midpoint)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for second chunk, got %d", resp.StatusCode)
	}

	written = getProgress(t, srv, sh.ID)
	if written != int64(len(largeData)) {
		t.Fatalf("expected %d bytes written after resume, got %d", len(largeData), written)
	}

	dlResp, err := http.Get(srv.URL + "/shares/" + sh.ID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", dlResp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(dlResp.Body)
	if err != nil {
		t.Fatal(err)
	}

	files := extractZipFiles(t, bodyBytes)
	if !bytes.Equal(files["sample.pdf"], largeData) {
		t.Fatalf("sample.pdf content mismatch: expected %d bytes, got %d bytes", len(largeData), len(files["sample.pdf"]))
	}
}

func TestBinaryIntegrity(t *testing.T) {
	srv := setupTestServer(t)

	binaryData := readFile(t, "binary.bin")

	body := handler.ShareRequest{
		Handle:        "binary-test",
		ExpiryMinutes: 60,
		Files: []handler.FileRequest{
			{Name: "binary.bin", ExpectedSize: int64(len(binaryData)), Order: 0},
		},
	}
	sh := createShare(t, srv, body)

	resp := streamFile(t, srv, sh.ID, sh.Files[0].ID, binaryData, 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	dlResp, err := http.Get(srv.URL + "/shares/" + sh.ID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", dlResp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(dlResp.Body)
	if err != nil {
		t.Fatal(err)
	}

	files := extractZipFiles(t, bodyBytes)
	if !bytes.Equal(files["binary.bin"], binaryData) {
		t.Fatalf("binary.bin round-trip failed: expected %d bytes, got %d bytes", len(binaryData), len(files["binary.bin"]))
	}
}

func TestDownloadIncompleteShare(t *testing.T) {
	srv := setupTestServer(t)

	binaryData := readFile(t, "binary.bin")
	partialSize := int64(len(binaryData) / 2)

	body := handler.ShareRequest{
		Handle:        "incomplete-test",
		ExpiryMinutes: 60,
		Files: []handler.FileRequest{
			{Name: "binary.bin", ExpectedSize: int64(len(binaryData)), Order: 0},
		},
	}
	sh := createShare(t, srv, body)

	resp := streamFile(t, srv, sh.ID, sh.Files[0].ID, binaryData[:partialSize], 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for partial upload, got %d", resp.StatusCode)
	}

	dlResp, err := http.Get(srv.URL + "/shares/" + sh.ID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for incomplete share download, got %d", dlResp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(dlResp.Body)
	if err != nil {
		t.Fatal(err)
	}

	files := extractZipFiles(t, bodyBytes)
	if len(files) != 1 {
		t.Fatalf("expected 1 file in zip, got %d", len(files))
	}

	if !bytes.Equal(files["binary.bin"], binaryData[:partialSize]) {
		t.Fatalf("partial content mismatch: expected %d bytes, got %d bytes", partialSize, len(files["binary.bin"]))
	}
}
