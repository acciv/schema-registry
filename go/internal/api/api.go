package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/acciv/schema-registry/internal/registry"
)

type Server struct {
	store *registry.Store
}

func New(store *registry.Store) *Server {
	return &Server{store: store}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func decode(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /stats", s.stats)
	mux.HandleFunc("POST /v1/check", s.check)
	mux.HandleFunc("GET /v1/schemas", s.list)
	mux.HandleFunc("POST /v1/schemas", s.register)
	mux.HandleFunc("GET /v1/schemas/{name}", s.versions)
	mux.HandleFunc("GET /v1/schemas/{name}/{version}", s.get)
	mux.HandleFunc("POST /v1/schemas/{name}/{version}/check", s.checkVersion)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"core":   registry.Version(),
		"store":  len(s.store.Names()),
	})
}

func (s *Server) stats(w http.ResponseWriter, _ *http.Request) {
	stats := s.store.CurrentStats()
	writeJSON(w, http.StatusOK, map[string]any{
		"registrations": stats.Registrations,
		"checks":        stats.Checks,
		"rejected":      stats.Rejected,
	})
}

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Old json.RawMessage `json:"old"`
		New json.RawMessage `json:"new"`
	}
	if err := decode(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	report, err := registry.Check(body.Old, body.New)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) list(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"names": s.store.Names()})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string          `json:"name"`
		Version       string          `json:"version"`
		Schema        json.RawMessage `json:"schema"`
		AllowBreaking bool            `json:"allow_breaking"`
	}
	if err := decode(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	entry, report, err := s.store.Register(body.Name, body.Version, body.Schema, body.AllowBreaking)
	if err != nil {
		if errors.Is(err, registry.ErrBreaking) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":  err.Error(),
				"report": report,
			})
			return
		}
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"entry": entry, "report": report})
}

func (s *Server) versions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	versions := s.store.Versions(name)
	if len(versions) == 0 {
		fail(w, http.StatusNotFound, "schema not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "versions": versions})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	entry, ok := s.store.Get(r.PathValue("name"), r.PathValue("version"))
	if !ok {
		fail(w, http.StatusNotFound, "version not found")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) checkVersion(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := decode(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	report, err := s.store.CheckAgainst(r.PathValue("name"), r.PathValue("version"), body.Schema)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			fail(w, http.StatusNotFound, err.Error())
			return
		}
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
