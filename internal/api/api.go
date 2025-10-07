package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dppeppel/scryarr/internal/config"
	"github.com/dppeppel/scryarr/internal/logging"
	"github.com/dppeppel/scryarr/internal/store"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	log = logging.GetLogger("api")
}

// Server represents the HTTP API server
type Server struct {
	store       *store.Store
	categories  *config.CategoriesConfig
	jsonOutDir  string
	pmmOutDir   string
	bindAddr    string
	triggerFunc func() error // Function to trigger a manual job run
}

// NewServer creates a new API server
func NewServer(
	store *store.Store,
	categories *config.CategoriesConfig,
	jsonOutDir string,
	pmmOutDir string,
	bindAddr string,
	triggerFunc func() error,
) *Server {
	return &Server{
		store:       store,
		categories:  categories,
		jsonOutDir:  jsonOutDir,
		pmmOutDir:   pmmOutDir,
		bindAddr:    bindAddr,
		triggerFunc: triggerFunc,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	r := mux.NewRouter()

	r.HandleFunc("/v1/health", s.handleHealth).Methods("GET")
	r.HandleFunc("/v1/categories", s.handleCategories).Methods("GET")
	r.HandleFunc("/v1/runs/latest", s.handleLatestRun).Methods("GET")
	r.HandleFunc("/v1/recs/{label}/latest", s.handleLatestRecs).Methods("GET")
	r.HandleFunc("/v1/recs/{label}/latest/raw", s.handleLatestRecsRaw).Methods("GET")
	r.HandleFunc("/v1/pmm/collections", s.handlePMMCollections).Methods("GET")
	r.HandleFunc("/v1/run", s.handleTriggerRun).Methods("POST")

	log.Info().Str("addr", s.bindAddr).Msg("starting API server")
	return http.ListenAndServe(s.bindAddr, r)
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *Server) sendError(w http.ResponseWriter, code int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	resp := ErrorResponse{}
	resp.Error.Code = errCode
	resp.Error.Message = message

	json.NewEncoder(w).Encode(resp)
}

func (s *Server) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	s.sendJSON(w, s.categories)
}

func (s *Server) handleLatestRun(w http.ResponseWriter, r *http.Request) {
	jobRun, err := s.store.GetLatestJobRun()
	if err != nil {
		s.sendError(w, 500, "internal_error", "Failed to fetch latest run")
		return
	}

	if jobRun == nil {
		s.sendError(w, 404, "not_found", "No job runs found")
		return
	}

	// Fetch category runs for this job
	catRuns, err := s.store.GetCategoryRunsByJobID(jobRun.ID)
	if err != nil {
		s.sendError(w, 500, "internal_error", "Failed to fetch category runs")
		return
	}

	response := map[string]interface{}{
		"job_run":       jobRun,
		"category_runs": catRuns,
	}

	s.sendJSON(w, response)
}

func (s *Server) handleLatestRecs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	label := vars["label"]

	catRun, err := s.store.GetLatestCategoryRun(label)
	if err != nil {
		s.sendError(w, 500, "internal_error", "Failed to fetch category run")
		return
	}

	if catRun == nil || catRun.ResolvedJSONPath == nil {
		s.sendError(w, 404, "not_found", "No recommendations found for this category")
		return
	}

	// Read the resolved JSON file
	data, err := os.ReadFile(*catRun.ResolvedJSONPath)
	if err != nil {
		s.sendError(w, 500, "internal_error", "Failed to read recommendations file")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handleLatestRecsRaw(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	label := vars["label"]

	catRun, err := s.store.GetLatestCategoryRun(label)
	if err != nil {
		s.sendError(w, 500, "internal_error", "Failed to fetch category run")
		return
	}

	if catRun == nil || catRun.RawJSONPath == nil {
		s.sendError(w, 404, "not_found", "No raw recommendations found for this category")
		return
	}

	// Read the raw JSON file
	data, err := os.ReadFile(*catRun.RawJSONPath)
	if err != nil {
		s.sendError(w, 500, "internal_error", "Failed to read raw recommendations file")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handlePMMCollections(w http.ResponseWriter, r *http.Request) {
	files, err := os.ReadDir(s.pmmOutDir)
	if err != nil {
		s.sendError(w, 500, "internal_error", "Failed to read PMM output directory")
		return
	}

	var collections []map[string]interface{}
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".yml" {
			continue
		}

		info, _ := file.Info()
		collections = append(collections, map[string]interface{}{
			"filename":  file.Name(),
			"path":      filepath.Join(s.pmmOutDir, file.Name()),
			"size":      info.Size(),
			"modified":  info.ModTime(),
		})
	}

	s.sendJSON(w, map[string]interface{}{
		"collections": collections,
	})
}

func (s *Server) handleTriggerRun(w http.ResponseWriter, r *http.Request) {
	if s.triggerFunc == nil {
		s.sendError(w, 503, "not_available", "Manual trigger not available in this mode")
		return
	}

	// Trigger the job run in a goroutine and return immediately
	go func() {
		if err := s.triggerFunc(); err != nil {
			log.Error().Err(err).Msg("manual job run failed")
		}
	}()

	s.sendJSON(w, map[string]string{
		"status":  "triggered",
		"message": "Job run initiated",
	})
}

// ReadFile is a helper to read a file and return its content
func ReadFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// WriteFile is a helper to write content to a file
func WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// ListYAMLFiles lists all YAML files in a directory
func ListYAMLFiles(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var yamlFiles []string
	for _, file := range files {
		if !file.IsDir() && (filepath.Ext(file.Name()) == ".yml" || filepath.Ext(file.Name()) == ".yaml") {
			yamlFiles = append(yamlFiles, file.Name())
		}
	}

	return yamlFiles, nil
}
