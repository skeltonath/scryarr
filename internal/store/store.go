package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store handles all database operations
type Store struct {
	db *sql.DB
}

// NewStore creates a new Store instance and initializes the schema
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &Store{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// initSchema creates all tables if they don't exist
func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS job_run (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at TEXT NOT NULL,
		finished_at TEXT,
		mode TEXT NOT NULL,
		status TEXT NOT NULL,
		error_msg TEXT
	);

	CREATE TABLE IF NOT EXISTS category_run (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER NOT NULL REFERENCES job_run(id),
		label TEXT NOT NULL,
		type TEXT NOT NULL,
		raw_json_path TEXT,
		resolved_json_path TEXT,
		pmm_movie_yaml_path TEXT,
		pmm_tv_yaml_path TEXT,
		status TEXT NOT NULL,
		error_msg TEXT
	);

	CREATE TABLE IF NOT EXISTS recommendation_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		label TEXT NOT NULL,
		tmdb_id INTEGER NOT NULL,
		media_type TEXT CHECK (media_type IN ('movie','tv')),
		first_seen_at TEXT NOT NULL,
		last_seen_at TEXT NOT NULL
	);
	CREATE UNIQUE INDEX IF NOT EXISTS ix_history_label_tmdb ON recommendation_history(label, tmdb_id, media_type);

	CREATE TABLE IF NOT EXISTS title_resolution_cache (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		year INTEGER,
		media_type TEXT CHECK (media_type IN ('movie','tv')),
		tmdb_id INTEGER,
		imdb_id TEXT,
		country TEXT,
		runtime_min INTEGER,
		resolved_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS plex_inventory (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tmdb_id INTEGER NOT NULL,
		media_type TEXT CHECK (media_type IN ('movie','tv')),
		present_at TEXT NOT NULL
	);
	CREATE UNIQUE INDEX IF NOT EXISTS ix_inventory_tmdb ON plex_inventory(tmdb_id, media_type);
	`

	_, err := s.db.Exec(schema)
	return err
}

// JobRun represents a job run record
type JobRun struct {
	ID         int64
	StartedAt  time.Time
	FinishedAt *time.Time
	Mode       string
	Status     string // running, completed, failed
	ErrorMsg   *string
}

// CreateJobRun creates a new job run record
func (s *Store) CreateJobRun(mode string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO job_run (started_at, mode, status) VALUES (?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339),
		mode,
		"running",
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateJobRun updates a job run record
func (s *Store) UpdateJobRun(id int64, status string, errorMsg *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		"UPDATE job_run SET finished_at = ?, status = ?, error_msg = ? WHERE id = ?",
		now, status, errorMsg, id,
	)
	return err
}

// GetLatestJobRun retrieves the most recent job run
func (s *Store) GetLatestJobRun() (*JobRun, error) {
	row := s.db.QueryRow("SELECT id, started_at, finished_at, mode, status, error_msg FROM job_run ORDER BY id DESC LIMIT 1")

	var jr JobRun
	var startedAt, finishedAt sql.NullString
	var errorMsg sql.NullString

	err := row.Scan(&jr.ID, &startedAt, &finishedAt, &jr.Mode, &jr.Status, &errorMsg)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	jr.StartedAt, _ = time.Parse(time.RFC3339, startedAt.String)
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339, finishedAt.String)
		jr.FinishedAt = &t
	}
	if errorMsg.Valid {
		s := errorMsg.String
		jr.ErrorMsg = &s
	}

	return &jr, nil
}

// CategoryRun represents a category run record
type CategoryRun struct {
	ID                 int64
	JobID              int64
	Label              string
	Type               string
	RawJSONPath        *string
	ResolvedJSONPath   *string
	PMMMovieYAMLPath   *string
	PMMTVYAMLPath      *string
	Status             string // running, completed, failed
	ErrorMsg           *string
}

// CreateCategoryRun creates a new category run record
func (s *Store) CreateCategoryRun(jobID int64, label, catType string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO category_run (job_id, label, type, status) VALUES (?, ?, ?, ?)",
		jobID, label, catType, "running",
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateCategoryRun updates a category run record
func (s *Store) UpdateCategoryRun(id int64, status string, paths map[string]*string, errorMsg *string) error {
	rawJSON := paths["raw_json"]
	resolvedJSON := paths["resolved_json"]
	pmmMovie := paths["pmm_movie"]
	pmmTV := paths["pmm_tv"]

	_, err := s.db.Exec(
		`UPDATE category_run
		SET status = ?, raw_json_path = ?, resolved_json_path = ?,
		    pmm_movie_yaml_path = ?, pmm_tv_yaml_path = ?, error_msg = ?
		WHERE id = ?`,
		status, rawJSON, resolvedJSON, pmmMovie, pmmTV, errorMsg, id,
	)
	return err
}

// GetCategoryRunsByJobID retrieves all category runs for a job
func (s *Store) GetCategoryRunsByJobID(jobID int64) ([]CategoryRun, error) {
	rows, err := s.db.Query(
		`SELECT id, job_id, label, type, raw_json_path, resolved_json_path,
		        pmm_movie_yaml_path, pmm_tv_yaml_path, status, error_msg
		FROM category_run WHERE job_id = ?`,
		jobID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []CategoryRun
	for rows.Next() {
		var cr CategoryRun
		var rawJSON, resolvedJSON, pmmMovie, pmmTV, errorMsg sql.NullString

		err := rows.Scan(&cr.ID, &cr.JobID, &cr.Label, &cr.Type,
			&rawJSON, &resolvedJSON, &pmmMovie, &pmmTV, &cr.Status, &errorMsg)
		if err != nil {
			return nil, err
		}

		if rawJSON.Valid {
			s := rawJSON.String
			cr.RawJSONPath = &s
		}
		if resolvedJSON.Valid {
			s := resolvedJSON.String
			cr.ResolvedJSONPath = &s
		}
		if pmmMovie.Valid {
			s := pmmMovie.String
			cr.PMMMovieYAMLPath = &s
		}
		if pmmTV.Valid {
			s := pmmTV.String
			cr.PMMTVYAMLPath = &s
		}
		if errorMsg.Valid {
			s := errorMsg.String
			cr.ErrorMsg = &s
		}

		runs = append(runs, cr)
	}

	return runs, rows.Err()
}

// GetLatestCategoryRun retrieves the most recent category run for a label
func (s *Store) GetLatestCategoryRun(label string) (*CategoryRun, error) {
	row := s.db.QueryRow(
		`SELECT id, job_id, label, type, raw_json_path, resolved_json_path,
		        pmm_movie_yaml_path, pmm_tv_yaml_path, status, error_msg
		FROM category_run WHERE label = ? ORDER BY id DESC LIMIT 1`,
		label,
	)

	var cr CategoryRun
	var rawJSON, resolvedJSON, pmmMovie, pmmTV, errorMsg sql.NullString

	err := row.Scan(&cr.ID, &cr.JobID, &cr.Label, &cr.Type,
		&rawJSON, &resolvedJSON, &pmmMovie, &pmmTV, &cr.Status, &errorMsg)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if rawJSON.Valid {
		s := rawJSON.String
		cr.RawJSONPath = &s
	}
	if resolvedJSON.Valid {
		s := resolvedJSON.String
		cr.ResolvedJSONPath = &s
	}
	if pmmMovie.Valid {
		s := pmmMovie.String
		cr.PMMMovieYAMLPath = &s
	}
	if pmmTV.Valid {
		s := pmmTV.String
		cr.PMMTVYAMLPath = &s
	}
	if errorMsg.Valid {
		s := errorMsg.String
		cr.ErrorMsg = &s
	}

	return &cr, nil
}

// RecordRecommendation records or updates a recommendation in history
func (s *Store) RecordRecommendation(label string, tmdbID int, mediaType string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Try insert first
	_, err := s.db.Exec(
		`INSERT INTO recommendation_history (label, tmdb_id, media_type, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(label, tmdb_id, media_type) DO UPDATE SET last_seen_at = ?`,
		label, tmdbID, mediaType, now, now, now,
	)
	return err
}

// GetRecommendationsSince retrieves recommendations seen since a given date
func (s *Store) GetRecommendationsSince(label string, since time.Time) (map[int]bool, error) {
	rows, err := s.db.Query(
		"SELECT tmdb_id FROM recommendation_history WHERE label = ? AND last_seen_at >= ?",
		label, since.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = true
	}

	return result, rows.Err()
}

// TitleResolution represents a cached title resolution
type TitleResolution struct {
	Title      string
	Year       int
	MediaType  string
	TMDbID     int
	IMDbID     string
	Country    string
	RuntimeMin int
}

// CacheTitleResolution stores a title resolution in cache
func (s *Store) CacheTitleResolution(tr *TitleResolution) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO title_resolution_cache
		(title, year, media_type, tmdb_id, imdb_id, country, runtime_min, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tr.Title, tr.Year, tr.MediaType, tr.TMDbID, tr.IMDbID, tr.Country, tr.RuntimeMin, now,
	)
	return err
}

// GetTitleResolution retrieves a cached title resolution
func (s *Store) GetTitleResolution(title string, year int, mediaType string) (*TitleResolution, error) {
	row := s.db.QueryRow(
		`SELECT title, year, media_type, tmdb_id, imdb_id, country, runtime_min
		FROM title_resolution_cache
		WHERE title = ? AND year = ? AND media_type = ?
		ORDER BY resolved_at DESC LIMIT 1`,
		title, year, mediaType,
	)

	var tr TitleResolution
	var imdbID, country sql.NullString
	var runtimeMin sql.NullInt64

	err := row.Scan(&tr.Title, &tr.Year, &tr.MediaType, &tr.TMDbID, &imdbID, &country, &runtimeMin)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if imdbID.Valid {
		tr.IMDbID = imdbID.String
	}
	if country.Valid {
		tr.Country = country.String
	}
	if runtimeMin.Valid {
		tr.RuntimeMin = int(runtimeMin.Int64)
	}

	return &tr, nil
}

// UpdatePlexInventory refreshes the Plex inventory table
func (s *Store) UpdatePlexInventory(items []struct{ TMDbID int; MediaType string }) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing inventory
	if _, err := tx.Exec("DELETE FROM plex_inventory"); err != nil {
		return err
	}

	// Insert new inventory
	stmt, err := tx.Prepare("INSERT INTO plex_inventory (tmdb_id, media_type, present_at) VALUES (?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, item := range items {
		if _, err := stmt.Exec(item.TMDbID, item.MediaType, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// IsInPlexInventory checks if a TMDb ID is in Plex inventory
func (s *Store) IsInPlexInventory(tmdbID int, mediaType string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM plex_inventory WHERE tmdb_id = ? AND media_type = ?",
		tmdbID, mediaType,
	).Scan(&count)
	return count > 0, err
}
