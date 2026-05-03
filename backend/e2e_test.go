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

type testClient struct {
	t            *testing.T
	url          string
	db           *sql.DB
	shareService *service.ShareService
}

func setup(t *testing.T) *testClient {
	t.Helper()
	tmpDir := t.TempDir()

	dbStorage := storage.NewDatabaseStorage(filepath.Join(tmpDir, "test.db"))
	fileStorage := storage.NewFileStorage(filepath.Join(tmpDir, "data"))
	shareRepo := repository.NewShareRepository(dbStorage)
	shareSvc := service.NewShareService(shareRepo, fileStorage)
	shareCtrl := handler.NewShareController(shareSvc)

	router := http.NewServeMux()
	shareCtrl.RegisterHandlers(router)

	srv := httptest.NewServer(middleware.Logging(router))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { dbStorage.DB.Close() })

	return &testClient{
		t:            t,
		url:          srv.URL,
		db:           dbStorage.DB,
		shareService: shareSvc,
	}
}

func (c *testClient) do(method, path string, body any) *http.Response {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.url+path, r)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func (c *testClient) decode(resp *http.Response, v any) {
	c.t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		c.t.Fatal(err)
	}
}

func (c *testClient) Default(handle string, expiryMinutes int) handler.ShareRequest {
	return handler.ShareRequest{
		Handle:        handle,
		ExpiryMinutes: expiryMinutes,
		Files: []handler.FileRequest{
			{Name: "test.txt", ExpectedSize: 100, Order: 0},
		},
	}
}

func (c *testClient) CreateShare(body handler.ShareRequest) share.Share {
	c.t.Helper()
	resp := c.do(http.MethodPost, "/shares", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var e api.Error
		c.decode(resp, &e)
		c.t.Fatalf("CreateShare: expected 201, got %d: %s", resp.StatusCode, e.Message)
	}

	var sh share.Share
	c.decode(resp, &sh)
	return sh
}

func (c *testClient) GetShare(id string) (*share.Share, int) {
	c.t.Helper()
	resp := c.do(http.MethodGet, "/shares/"+id, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode
	}
	var sh share.Share
	c.decode(resp, &sh)
	return &sh, resp.StatusCode
}

func (c *testClient) GetShares() []share.ShareSummary {
	c.t.Helper()
	resp := c.do(http.MethodGet, "/shares", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("GetShares: expected 200, got %d", resp.StatusCode)
	}
	var shares []share.ShareSummary
	c.decode(resp, &shares)
	return shares
}

func (c *testClient) DeleteShare(id string) int {
	c.t.Helper()
	resp := c.do(http.MethodDelete, "/shares/"+id, nil)
	defer resp.Body.Close()
	return resp.StatusCode
}

func (c *testClient) Progress(id string) int64 {
	c.t.Helper()
	resp := c.do(http.MethodGet, "/shares/"+id+"/progress", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("Progress: expected 200, got %d", resp.StatusCode)
	}
	var result map[string]int64
	c.decode(resp, &result)
	return result["bytesWritten"]
}

func (c *testClient) StreamFile(shareID, fileID string, data []byte, offset int64) *http.Response {
	c.t.Helper()
	url := fmt.Sprintf("%s/shares/%s/files/%s/stream", c.url, shareID, fileID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Write-Offset", strconv.FormatInt(offset, 10))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "resources", "e2e_tests", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
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
	client := setup(t)
	sh := client.CreateShare(client.Default("expiry-test", 60))

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
	client := setup(t)
	sh := client.CreateShare(client.Default("permanent-test", 0))

	if sh.ExpiresAt != nil {
		t.Fatalf("expected expiresAt to be null, got %v", *sh.ExpiresAt)
	}
}

func TestCreateShareInvalidExpiry(t *testing.T) {
	client := setup(t)
	resp := client.do(http.MethodPost, "/shares", client.Default("bad-expiry", -1))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateShareEmptyHandle(t *testing.T) {
	client := setup(t)
	resp := client.do(http.MethodPost, "/shares", client.Default("", 60))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateShareDuplicateHandle(t *testing.T) {
	client := setup(t)
	client.CreateShare(client.Default("dup-handle", 60))

	resp := client.do(http.MethodPost, "/shares", client.Default("dup-handle", 60))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestStreamFile(t *testing.T) {
	client := setup(t)
	sh := client.CreateShare(client.Default("stream-test", 60))

	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}

	resp := client.StreamFile(sh.ID, sh.Files[0].ID, data, 0)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestStreamFileInvalidOffset(t *testing.T) {
	client := setup(t)
	sh := client.CreateShare(client.Default("offset-test", 60))

	resp := client.StreamFile(sh.ID, sh.Files[0].ID, make([]byte, 10), 200)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestStreamFileNonExistentShare(t *testing.T) {
	client := setup(t)
	resp := client.StreamFile("nonexistent-id", "fake-file-id", make([]byte, 10), 0)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetShare(t *testing.T) {
	client := setup(t)
	created := client.CreateShare(client.Default("get-test", 60))

	got, status := client.GetShare(created.ID)
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
	client := setup(t)
	client.CreateShare(client.Default("list-test-1", 60))
	client.CreateShare(client.Default("list-test-2", 60))

	if shares := client.GetShares(); len(shares) < 2 {
		t.Fatalf("expected at least 2 shares, got %d", len(shares))
	}
}

func TestGetShareNotFound(t *testing.T) {
	client := setup(t)
	_, status := client.GetShare("nonexistent-id")
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", status)
	}
}

func TestGetProgress(t *testing.T) {
	client := setup(t)
	sh := client.CreateShare(client.Default("progress-test", 60))

	data := make([]byte, 50)
	for i := range data {
		data[i] = byte(i)
	}

	resp := client.StreamFile(sh.ID, sh.Files[0].ID, data, 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	if written := client.Progress(sh.ID); written != 50 {
		t.Fatalf("expected 50 bytes written, got %d", written)
	}
}

func TestDeleteShare(t *testing.T) {
	client := setup(t)
	sh := client.CreateShare(client.Default("delete-test", 60))

	if status := client.DeleteShare(sh.ID); status != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", status)
	}
	_, getStatus := client.GetShare(sh.ID)
	if getStatus != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getStatus)
	}
}

func TestDeleteNonExistentShare(t *testing.T) {
	client := setup(t)
	if status := client.DeleteShare("nonexistent-id"); status != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", status)
	}
}

func TestExpiryCleanup(t *testing.T) {
	client := setup(t)
	sh := client.CreateShare(client.Default("expiring", 60))

	past := time.Now().UTC().Add(-90 * time.Second).Format(time.RFC3339Nano)
	if _, err := client.db.Exec("UPDATE share SET expires_at = ? WHERE id = ?", past, sh.ID); err != nil {
		t.Fatal(err)
	}

	ids, err := client.shareService.GetExpiredShares()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("expected expired shares, got none")
	}
	if err = client.shareService.Delete(ids...); err != nil {
		t.Fatal(err)
	}

	_, status := client.GetShare(sh.ID)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 after expiry cleanup, got %d", status)
	}
}

func TestPermanentShareNotExpired(t *testing.T) {
	client := setup(t)
	client.CreateShare(client.Default("permanent", 0))

	ids, err := client.shareService.GetExpiredShares()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no expired shares, got %d", len(ids))
	}
}

func TestDeleteMultipleShares(t *testing.T) {
	client := setup(t)
	sh1 := client.CreateShare(client.Default("batch-1", 0))
	sh2 := client.CreateShare(client.Default("batch-2", 0))

	if err := client.shareService.Delete(sh1.ID, sh2.ID); err != nil {
		t.Fatal(err)
	}

	if _, status := client.GetShare(sh1.ID); status != http.StatusNotFound {
		t.Fatalf("expected 404 for first share, got %d", status)
	}
	if _, status := client.GetShare(sh2.ID); status != http.StatusNotFound {
		t.Fatalf("expected 404 for second share, got %d", status)
	}
}

func TestDownloadCompleteShare(t *testing.T) {
	client := setup(t)

	helloData := readFile(t, "hello.txt")
	binaryData := readFile(t, "binary.bin")

	sh := client.CreateShare(handler.ShareRequest{
		Handle:        "download-test",
		ExpiryMinutes: 60,
		Files: []handler.FileRequest{
			{Name: "hello.txt", ExpectedSize: int64(len(helloData)), Order: 0},
			{Name: "binary.bin", ExpectedSize: int64(len(binaryData)), Order: 1},
		},
	})

	for _, tc := range []struct {
		id   string
		data []byte
		name string
	}{
		{sh.Files[0].ID, helloData, "hello.txt"},
		{sh.Files[1].ID, binaryData, "binary.bin"},
	} {
		resp := client.StreamFile(sh.ID, tc.id, tc.data, 0)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("stream %s: expected 204, got %d", tc.name, resp.StatusCode)
		}
	}

	dlResp, err := http.Get(client.url + "/shares/" + sh.ID + "/download")
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
		t.Fatalf("hello.txt content mismatch: expected %d bytes, got %d", len(helloData), len(files["hello.txt"]))
	}
	if !bytes.Equal(files["binary.bin"], binaryData) {
		t.Fatalf("binary.bin content mismatch: expected %d bytes, got %d", len(binaryData), len(files["binary.bin"]))
	}
}

func TestStreamWithResume(t *testing.T) {
	client := setup(t)

	largeData := readFile(t, "sample.pdf")
	midpoint := int64(len(largeData) / 2)

	sh := client.CreateShare(handler.ShareRequest{
		Handle:        "resume-test",
		ExpiryMinutes: 60,
		Files: []handler.FileRequest{
			{Name: "sample.pdf", ExpectedSize: int64(len(largeData)), Order: 0},
		},
	})

	resp := client.StreamFile(sh.ID, sh.Files[0].ID, largeData[:midpoint], 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first chunk: expected 204, got %d", resp.StatusCode)
	}
	if written := client.Progress(sh.ID); written != midpoint {
		t.Fatalf("expected %d bytes after first chunk, got %d", midpoint, written)
	}

	resp = client.StreamFile(sh.ID, sh.Files[0].ID, largeData[midpoint:], midpoint)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second chunk: expected 204, got %d", resp.StatusCode)
	}
	if written := client.Progress(sh.ID); written != int64(len(largeData)) {
		t.Fatalf("expected %d bytes after resume, got %d", len(largeData), written)
	}

	dlResp, err := http.Get(client.url + "/shares/" + sh.ID + "/download")
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
		t.Fatalf("sample.pdf mismatch: expected %d bytes, got %d", len(largeData), len(files["sample.pdf"]))
	}
}

func TestBinaryIntegrity(t *testing.T) {
	client := setup(t)
	binaryData := readFile(t, "binary.bin")

	sh := client.CreateShare(handler.ShareRequest{
		Handle:        "binary-test",
		ExpiryMinutes: 60,
		Files: []handler.FileRequest{
			{Name: "binary.bin", ExpectedSize: int64(len(binaryData)), Order: 0},
		},
	})

	resp := client.StreamFile(sh.ID, sh.Files[0].ID, binaryData, 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	dlResp, err := http.Get(client.url + "/shares/" + sh.ID + "/download")
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
		t.Fatalf("binary.bin round-trip failed: expected %d bytes, got %d", len(binaryData), len(files["binary.bin"]))
	}
}

func TestDownloadIncompleteShare(t *testing.T) {
	client := setup(t)
	binaryData := readFile(t, "binary.bin")
	partialSize := int64(len(binaryData) / 2)

	sh := client.CreateShare(handler.ShareRequest{
		Handle:        "incomplete-test",
		ExpiryMinutes: 60,
		Files: []handler.FileRequest{
			{Name: "binary.bin", ExpectedSize: int64(len(binaryData)), Order: 0},
		},
	})

	resp := client.StreamFile(sh.ID, sh.Files[0].ID, binaryData[:partialSize], 0)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for partial upload, got %d", resp.StatusCode)
	}

	dlResp, err := http.Get(client.url + "/shares/" + sh.ID + "/download")
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
		t.Fatalf("partial content mismatch: expected %d bytes, got %d", partialSize, len(files["binary.bin"]))
	}
}
