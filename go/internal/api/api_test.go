package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acciv/schema-registry/internal/registry"
)

const v1 = `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`
const v2 = `{"type":"object","properties":{"id":{"type":"string"},"page":{"type":"integer"}},"required":["id"]}`
const v3 = `{"type":"object","properties":{"id":{"type":"string"},"token":{"type":"string"}},"required":["id","token"]}`

func testServer(t *testing.T) http.Handler {
	t.Helper()
	store, err := registry.New(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	return New(store).Routes()
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRegisterAndRead(t *testing.T) {
	h := testServer(t)
	body := `{"name":"user.created","version":"1.0.0","schema":` + v1 + `}`
	if rec := post(t, h, "/v1/schemas", body); rec.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/v1/schemas"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "user.created") {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/v1/schemas/user.created/1.0.0"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "fingerprint") {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/v1/schemas/user.created"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "versions") {
		t.Fatalf("versions: %d %s", rec.Code, rec.Body.String())
	}
}

func TestBreakingIsConflict(t *testing.T) {
	h := testServer(t)
	_ = post(t, h, "/v1/schemas", `{"name":"u","version":"1.0.0","schema":`+v1+`}`)
	rec := post(t, h, "/v1/schemas", `{"name":"u","version":"2.0.0","schema":`+v3+`}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Report struct {
			Compatible bool `json:"compatible"`
		} `json:"report"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Report.Compatible {
		t.Fatalf("bad conflict body: %s", rec.Body.String())
	}
}

func TestCheckEndpoints(t *testing.T) {
	h := testServer(t)
	_ = post(t, h, "/v1/schemas", `{"name":"u","version":"1.0.0","schema":`+v1+`}`)

	rec := post(t, h, "/v1/check", `{"old":`+v1+`,"new":`+v2+`}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"compatible":true`) {
		t.Fatalf("check: %d %s", rec.Code, rec.Body.String())
	}

	rec = post(t, h, "/v1/schemas/u/1.0.0/check", `{"schema":`+v3+`}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"compatible":false`) {
		t.Fatalf("check version: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHealthAndStats(t *testing.T) {
	h := testServer(t)
	if rec := get(t, h, "/healthz"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("health: %d %s", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/stats"); rec.Code != 200 {
		t.Fatalf("stats: %d", rec.Code)
	}
}
