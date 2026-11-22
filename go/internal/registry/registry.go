package registry

/*
#cgo CFLAGS: -I${SRCDIR}/../../../rust/include
#cgo LDFLAGS: -L${SRCDIR}/../../../rust/target/release -lschemar_core -Wl,-rpath,${SRCDIR}/../../../rust/target/release
#include <stdlib.h>
#include "schemar_core.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"unsafe"
)

type Entry struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Schema      json.RawMessage `json:"schema"`
	Fingerprint string          `json:"fingerprint"`
	CreatedAt   string          `json:"created_at"`
}

type Report struct {
	Compatible bool             `json:"compatible"`
	Breaking   int              `json:"breaking"`
	Changes    []map[string]any `json:"changes"`
}

var ErrBreaking = errors.New("breaking change rejected")

type Stats struct {
	Registrations int
	Checks        int
	Rejected      int
}

type Store struct {
	mu      sync.RWMutex
	path    string
	entries map[string][]Entry
	stats   Stats
}

func New(path string) (*Store, error) {
	store := &Store{path: path, entries: map[string][]Entry{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return store, nil
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode registry file: %w", err)
	}
	for _, entry := range entries {
		store.entries[entry.Name] = append(store.entries[entry.Name], entry)
	}
	return store, nil
}

func (s *Store) saveLocked() error {
	all := make([]Entry, 0)
	for _, versions := range s.entries {
		all = append(all, versions...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Name == all[j].Name {
			return all[i].Version < all[j].Version
		}
		return all[i].Name < all[j].Name
	})
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func Version() string {
	return C.GoString(C.schemar_version())
}

func fingerprint(schema json.RawMessage) (string, error) {
	ptr := C.CString(string(schema))
	defer C.free(unsafe.Pointer(ptr))
	out := C.schemar_fingerprint(ptr)
	if out == nil {
		return "", errors.New("nil from core")
	}
	defer C.schemar_free(out)
	var result struct {
		Fingerprint string `json:"fingerprint"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal([]byte(C.GoString(out)), &result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", errors.New(result.Error)
	}
	return result.Fingerprint, nil
}

// Check сравнивает две схемы через Rust-ядро.
func Check(oldSchema, newSchema json.RawMessage) (*Report, error) {
	oldPtr := C.CString(string(oldSchema))
	newPtr := C.CString(string(newSchema))
	defer C.free(unsafe.Pointer(oldPtr))
	defer C.free(unsafe.Pointer(newPtr))

	out := C.schemar_check(oldPtr, newPtr)
	if out == nil {
		return nil, errors.New("nil from core")
	}
	defer C.schemar_free(out)

	var report Report
	var raw struct {
		Compatible *bool             `json:"compatible"`
		Breaking   int               `json:"breaking"`
		Changes    []map[string]any  `json:"changes"`
		Error      string            `json:"error"`
	}
	if err := json.Unmarshal([]byte(C.GoString(out)), &raw); err != nil {
		return nil, fmt.Errorf("decode core report: %w", err)
	}
	if raw.Error != "" {
		return nil, errors.New(raw.Error)
	}
	if raw.Compatible == nil {
		return nil, errors.New("core returned no verdict")
	}
	report.Compatible = *raw.Compatible
	report.Breaking = raw.Breaking
	if raw.Changes == nil {
		report.Changes = []map[string]any{}
	} else {
		report.Changes = raw.Changes
	}
	return &report, nil
}

func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.entries))
	for name := range s.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Store) Versions(name string) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	versions := append([]Entry(nil), s.entries[name]...)
	return versions
}

func (s *Store) Get(name, version string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, entry := range s.entries[name] {
		if entry.Version == version {
			return entry, true
		}
	}
	return Entry{}, false
}

func (s *Store) Latest(name string) (Entry, bool) {
	versions := s.Versions(name)
	if len(versions) == 0 {
		return Entry{}, false
	}
	return versions[len(versions)-1], true
}

func (s *Store) CurrentStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// CheckAgainst сравнивает схему с сохранённой версией, не меняя хранилище.
func (s *Store) CheckAgainst(name, version string, schema json.RawMessage) (*Report, error) {
	entry, ok := s.Get(name, version)
	if !ok {
		return nil, fmt.Errorf("schema %s@%s not found", name, version)
	}
	s.mu.Lock()
	s.stats.Checks++
	s.mu.Unlock()
	return Check(entry.Schema, schema)
}

// Register добавляет версию схемы. По умолчанию breaking changes отклоняются.
func (s *Store) Register(name, version string, schema json.RawMessage, allowBreaking bool) (Entry, *Report, error) {
	if name == "" || version == "" {
		return Entry{}, nil, errors.New("name and version are required")
	}
	var probe any
	if err := json.Unmarshal(schema, &probe); err != nil {
		return Entry{}, nil, fmt.Errorf("invalid schema JSON: %w", err)
	}
	fp, err := fingerprint(schema)
	if err != nil {
		return Entry{}, nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var report *Report
	if latest, ok := s.latestLocked(name); ok {
		report, err = Check(latest.Schema, schema)
		if err != nil {
			return Entry{}, nil, err
		}
		if !report.Compatible && !allowBreaking {
			s.stats.Rejected++
			return Entry{}, report, ErrBreaking
		}
	} else {
		report = &Report{Compatible: true, Changes: []map[string]any{}}
	}
	if _, ok := s.getLocked(name, version); ok {
		return Entry{}, report, fmt.Errorf("version %s already exists", version)
	}

	entry := Entry{
		Name:        name,
		Version:     version,
		Schema:      schema,
		Fingerprint: fp,
		CreatedAt:   nowString(),
	}
	s.entries[name] = append(s.entries[name], entry)
	s.stats.Registrations++
	if err := s.saveLocked(); err != nil {
		s.entries[name] = s.entries[name][:len(s.entries[name])-1]
		s.stats.Registrations--
		return Entry{}, report, err
	}
	return entry, report, nil
}

func (s *Store) latestLocked(name string) (Entry, bool) {
	versions := s.entries[name]
	if len(versions) == 0 {
		return Entry{}, false
	}
	return versions[len(versions)-1], true
}

func (s *Store) getLocked(name, version string) (Entry, bool) {
	for _, entry := range s.entries[name] {
		if entry.Version == version {
			return entry, true
		}
	}
	return Entry{}, false
}
