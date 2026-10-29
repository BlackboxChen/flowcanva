package main

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func collaborationTestHandler(t *testing.T) (*Handler, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := initDB(filepath.Join(t.TempDir(), "test.db"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := &Handler{db: db, log: logger}
	_, err = db.Exec(`INSERT INTO canvases(id,name) VALUES('cv','test')`)
	if err != nil {
		t.Fatal(err)
	}
	return h, setupRouter(h, Config{})
}
func testNode(t *testing.T, h *Handler, id, kind, raw string) {
	t.Helper()
	if _, err := h.db.Exec(`INSERT INTO nodes(id,canvas_id,node_type,content) VALUES(?,'cv',?,?)`, id, kind, raw); err != nil {
		t.Fatal(err)
	}
}
func testRequest(router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
func TestCollaborationRejectsStaleEditor(t *testing.T) {
	h, r := collaborationTestHandler(t)
	testNode(t, h, "n", "text", "original")
	a := testRequest(r, "PUT", "/api/canvases/cv/nodes/n", map[string]any{"content": "A edits", "expected_content": "original"})
	b := testRequest(r, "PUT", "/api/canvases/cv/nodes/n", map[string]any{"content": "B edits", "expected_content": "original"})
	if a.Code != 200 || b.Code != 409 {
		t.Fatalf("A=%d %s B=%d %s", a.Code, a.Body, b.Code, b.Body)
	}
	var content string
	h.db.QueryRow(`SELECT content FROM nodes WHERE id='n'`).Scan(&content)
	if content != "A edits" {
		t.Fatal("stale write replaced A")
	}
}
func TestCollaborationDuplicateAndIndependentConcurrentSubmissions(t *testing.T) {
	h, r := collaborationTestHandler(t)
	var calls atomic.Int32
	entered := make(chan bool, 2)
	release := make(chan bool)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		entered <- true
		<-release
		var data map[string]any
		json.NewDecoder(req.Body).Decode(&data)
		json.NewEncoder(w).Encode(map[string]any{"prompt_id": data["prompt_id"]})
	}))
	defer upstream.Close()
	for _, id := range []string{"a", "b", "c"} {
		testNode(t, h, id, "workflow", "{}")
	}
	body := func(id string) map[string]any {
		return map[string]any{"canvas_id": "cv", "node_id": id, "server_url": upstream.URL, "workflow_json": map[string]any{"1": map[string]any{"class_type": "Fake", "inputs": map[string]any{}}}}
	}
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 2)
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			responses <- testRequest(r, "POST", "/api/comfyui/execute", body(id))
		}(id)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			close(release)
			t.Fatal("independent submissions did not overlap")
		}
	}
	duplicate := testRequest(r, "POST", "/api/comfyui/execute", body("a"))
	overflow := testRequest(r, "POST", "/api/comfyui/execute", body("c"))
	close(release)
	wg.Wait()
	if duplicate.Code != 200 || !bytes.Contains(duplicate.Body.Bytes(), []byte(`"reused":true`)) {
		t.Fatalf("duplicate %d %s", duplicate.Code, duplicate.Body)
	}
	if overflow.Code != 429 || calls.Load() != 2 {
		t.Fatalf("limit=%d calls=%d", overflow.Code, calls.Load())
	}
	for i := 0; i < 2; i++ {
		if res := <-responses; res.Code != 200 {
			t.Fatalf("%d %s", res.Code, res.Body)
		}
	}
}
func TestCollaborationTrashRetainsFileReference(t *testing.T) {
	h, r := collaborationTestHandler(t)
	_, err := h.db.Exec(`INSERT INTO assets(id,url) VALUES('asset','/uploads/keep.png')`)
	if err != nil {
		t.Fatal(err)
	}
	testNode(t, h, "n", "image", `{"url":"/uploads/keep.png"}`)
	if res := testRequest(r, "DELETE", "/api/admin/assets/asset", nil); res.Code != 204 {
		t.Fatal(res.Body.String())
	}
	active := testRequest(r, "GET", "/api/admin/assets", nil)
	trash := testRequest(r, "GET", "/api/admin/assets?trash=true", nil)
	if bytes.Contains(active.Body.Bytes(), []byte(`"id":"asset"`)) || !bytes.Contains(trash.Body.Bytes(), []byte(`"id":"asset"`)) {
		t.Fatal("trash visibility incorrect")
	}
	refs := testRequest(r, "GET", "/api/admin/assets/asset/references", nil)
	if !bytes.Contains(refs.Body.Bytes(), []byte(`"node_id":"n"`)) {
		t.Fatal("lost reference")
	}
	if res := testRequest(r, "POST", "/api/admin/assets/asset/restore", nil); res.Code != 204 {
		t.Fatal("restore failed")
	}
}
