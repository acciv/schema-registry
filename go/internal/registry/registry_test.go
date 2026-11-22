package registry

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

const v1 = `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`
const v2 = `{"type":"object","properties":{"id":{"type":"string"},"page":{"type":"integer"}},"required":["id"]}`
const v3 = `{"type":"object","properties":{"id":{"type":"string"},"token":{"type":"string"}},"required":["id","token"]}`

func newStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(filepath.Join(t.TempDir(), "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestRegisterFlow(t *testing.T) {
	store := newStore(t)

	first, report, err := store.Register("user.created", "1.0.0", json.RawMessage(v1), false)
	if err != nil || !report.Compatible {
		t.Fatalf("first register: %v %+v", err, report)
	}
	if first.Fingerprint == "" {
		t.Fatal("empty fingerprint")
	}

	second, report, err := store.Register("user.created", "1.1.0", json.RawMessage(v2), false)
	if err != nil || !report.Compatible {
		t.Fatalf("compatible register: %v %+v", err, report)
	}
	if second.Fingerprint == first.Fingerprint {
		t.Fatal("fingerprint did not change")
	}

	_, report, err = store.Register("user.created", "2.0.0", json.RawMessage(v3), false)
	if !errors.Is(err, ErrBreaking) || report.Compatible {
		t.Fatalf("breaking change must be rejected: %v %+v", err, report)
	}

	_, report, err = store.Register("user.created", "2.0.0", json.RawMessage(v3), true)
	if err != nil || report.Compatible {
		t.Fatalf("forced register: %v %+v", err, report)
	}
	if latest, ok := store.Latest("user.created"); !ok || latest.Version != "2.0.0" {
		t.Fatalf("bad latest: %+v", latest)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	store, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Register("ping", "1.0.0", json.RawMessage(v1), false); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get("ping", "1.0.0"); !ok {
		t.Fatal("entry not reloaded")
	}
}

func TestCheckAgainst(t *testing.T) {
	store := newStore(t)
	if _, _, err := store.Register("ping", "1.0.0", json.RawMessage(v1), false); err != nil {
		t.Fatal(err)
	}
	report, err := store.CheckAgainst("ping", "1.0.0", json.RawMessage(v3))
	if err != nil {
		t.Fatal(err)
	}
	if report.Compatible {
		t.Fatal("expected breaking report")
	}
	if len(store.Names()) != 1 {
		t.Fatal("check changed the store")
	}
}

func TestBadJSON(t *testing.T) {
	store := newStore(t)
	if _, _, err := store.Register("x", "1.0.0", json.RawMessage("nope"), false); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
