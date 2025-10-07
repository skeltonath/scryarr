# PlexRecs MVP Design Document
**Type:** Local Dockerized Go Service  
**Audience:** Claude (Implementation Instruction)  
**Version:** MVP-Final (v1.0)

---

## 1. Overview

Implement a local service named **PlexRecs**.  
Its purpose is to automatically generate movie and TV recommendations for a local Plex library based on Plex watch history (via Tautulli). The service will use metadata from TMDb/TVDb and request recommendation candidates from an LLM.

You must build the service in Go as a single Dockerized binary.  
It must persist state in SQLite, output structured JSON results, and emit Plex Meta Manager (PMM) YAML files for Plex consumption.  
It must also expose a **local-only HTTP API** (no auth, no CORS) to support a future UI.

---

## 2. Architecture

Implement a single Go binary structured with these packages:

| Package | Responsibility |
|----------|----------------|
| `cmd/worker` | Entry point, job orchestration, scheduling, and API startup |
| `internal/tautulli` | Fetches recent Plex watch history |
| `internal/tmdb` | Resolves titles, retrieves metadata, genres, and keywords |
| `internal/plex` | Queries Plex for current inventory |
| `internal/llm` | Sends structured JSON prompts to the LLM endpoint and parses responses |
| `internal/resolve` | Resolves recommendations to TMDb IDs, enriches, deduplicates |
| `internal/publish` | Writes resolved JSON and PMM YAML files |
| `internal/store` | Manages SQLite persistence |
| `internal/api` | Implements the local HTTP API |

---

## 3. Runtime and Deployment

**Environment:**
- Single containerized Go binary
- Run as a local service (e.g., Unraid, Docker Compose)
- No external web exposure or cloud dependencies

**Stack:**
- Go 1.22+
- SQLite (mattn/go-sqlite3)
- YAML (goccy/go-yaml)
- HTTP Router (gorilla/mux)
- Scheduler (robfig/cron/v3)
- Logging (rs/zerolog)
- LLM (sashabaranov/go-openai or equivalent)

**Volumes:**
| Path | Purpose |
|------|----------|
| `/config` | YAML configuration files |
| `/data` | SQLite database and JSON output files |
| `/output` | PMM YAML collection files |

---

## 4. Configuration

### `/config/app.yml`

Use YAML for application-level configuration. Example:

```yaml
app:
  mode: oneshot              # oneshot | loop
  schedule_cron: "0 3 * * *" # used if mode=loop
  log_level: info

paths:
  db_path: /data/plexrecs.sqlite
  json_out_dir: /data/recommendations
  pmm_out_dir: /output

tautulli:
  url: "https://tautulli.local"
  lookback_days: 120

plex:
  url: "https://plex.local"

recommender:
  model: "gpt-4o-mini"
  recs_per_category: 20
  diversity_min_fraction: 0.3
  recency_weight: 0.6
  allow_media_types: ["movie","tv"]

overseerr:
  enabled: false
  url: "https://overseerr.local"
  requests_per_category: 0

api:
  enabled: true
  bind_addr: "0.0.0.0:8080"
```

### `/config/categories.yml`

Implement a flexible category system. Each entry defines how to frame a recommendation query.

```yaml
categories:
  - label: "True Crime"
    type: "genre"
    media_types: ["movie","tv"]
    tmdb_filters:
      include_genres: ["Documentary","Crime"]
    keywords_prefer: ["true crime","investigative journalism","fraud","forensics","cold case"]
    keywords_avoid: ["mockumentary"]

  - label: "Because — The Jinx"
    type: "title_seed"
    media_types: ["tv","movie"]
    seed:
      title: "The Jinx: The Life and Deaths of Robert Durst"
      year: 2015
      medium: "tv"

  - label: "Cozy"
    type: "keyword"
    media_types: ["movie","tv"]
    mood_keywords: ["cozy","gentle","uplifting","low-stakes"]
    tmdb_filters:
      exclude_genres: ["Horror","War"]

  - label: "DP Favorites — Crime Seeds"
    type: "seed_list"
    media_types: ["movie","tv"]
    seeds:
      - { title: "The Imposter", year: 2012, medium: "movie" }
      - { title: "Murder on Middle Beach", year: 2020, medium: "tv" }
```

### Environment Variables

```
TAUTULLI_API_KEY=...
PLEX_TOKEN=...
TMDB_API_KEY=...
TVDB_API_KEY=...
LLM_API_BASE=https://api.openai-like-gateway.example/v1
LLM_API_KEY=...
```

---

## 5. Database Schema (SQLite)

Implement the schema exactly as follows:

```sql
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
```

---

## 6. LLM Prompt & Output

Implement a structured JSON prompt workflow.

**System message:**
> You are a recommender for a private media server. Suggest items constrained by the provided category and constraints. Return strict JSON matching the schema. Do not include already_seen or already_recommended titles. No streaming or acquisition info.

**User payload example:**
```json
{
  "task": "recommend",
  "category": { "label": "True Crime", "type": "genre", "tmdb_filters": { "include_genres": ["Documentary","Crime"] } },
  "constraints": { "count": 20, "recency_weight": 0.6, "diversity_min_fraction": 0.3 },
  "taste_profile": { "top_likes": [...], "recent_watches": [...] },
  "already_seen": [...],
  "already_recommended": [...],
  "output_schema": { "...as defined..." }
}
```

**LLM Response → Raw JSON:**
```json
{
  "category": "True Crime",
  "generated_at": "2025-10-07T03:00:03Z",
  "recommendations": [
    {
      "title": "Murder in the Outback",
      "year": 2020,
      "medium": "tv",
      "why": "Explores a wrongful conviction and media bias.",
      "keywords": ["investigative","australia"]
    }
  ]
}
```

**Resolver Output → Resolved JSON:**
```json
{
  "category": "True Crime",
  "resolved_at": "2025-10-07T03:00:16Z",
  "items": [
    {
      "title": "Murder in the Outback",
      "year": 2020,
      "medium": "tv",
      "tmdb_id": 123456,
      "imdb_id": "tt1234567",
      "runtime_min": 48,
      "vote_count": 842,
      "why": "Explores a wrongful conviction and media bias.",
      "keywords": ["investigative","australia"]
    }
  ]
}
```

**PMM YAML Output:**
```yaml
collections:
  "!01_Recommended — True Crime (Series)":
    tmdb_show:
      - 123456
    sync_mode: replace
    summary: "AI-curated picks based on Plex history & category prefs."
```

---

## 7. API Design (Local Only)

**Bind:** `0.0.0.0:8080`  
**Auth:** None  
**CORS:** None  

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/health` | Health check |
| `GET` | `/v1/categories` | List configured categories |
| `GET` | `/v1/runs/latest` | Latest job run with per-category statuses |
| `GET` | `/v1/recs/{label}/latest` | Latest resolved recs for a category |
| `GET` | `/v1/recs/{label}/latest/raw` | Latest raw LLM output |
| `GET` | `/v1/pmm/collections` | List emitted PMM YAMLs |
| `POST` | `/v1/run` | Trigger a manual job run (blocking, one at a time) |

**Error format:**
```json
{ "error": { "code": "bad_request", "message": "explanation" } }
```

**Implementation:**  
Use `gorilla/mux`. Example:

```go
r := mux.NewRouter()
r.HandleFunc("/v1/health", handleHealth).Methods("GET")
r.HandleFunc("/v1/recs/{label}/latest", handleLatestRecs).Methods("GET")
http.ListenAndServe(cfg.API.BindAddr, r)
```

---

## 8. Component Responsibilities

- `internal/tautulli`: implement `GetHistory()` to call `/api/v2?cmd=get_history`.  
- `internal/tmdb`: implement `SearchMovie()`, `SearchTV()`, `GetDetails()`, `GetKeywords()`.  
- `internal/plex`: implement `GetInventory()` using `/library/all`.  
- `internal/llm`: implement `SendPrompt(payload)` calling `/v1/chat/completions`.  
- `internal/resolve`: merge LLM output with TMDb results, attach IDs, filter duplicates.  
- `internal/publish`: output resolved JSON, then generate PMM YAML.  
- `internal/store`: provide typed DAO wrappers for each table.  
- `internal/api`: expose endpoints, read from SQLite and JSON outputs.  

---

## 9. Execution Modes

**oneshot:** Run one full job and exit.  
**loop:** Start internal scheduler via `robfig/cron`, keep API active.

---

## 10. Development Notes

- No external exposure; local-only server.  
- Use gorilla/mux, goccy/go-yaml, mattn/go-sqlite3.  
- Ensure PMM YAMLs always reference TMDb IDs.  
- Deduplicate via `recommendation_history` and `plex_inventory`.  
- Log using zerolog (structured JSON logs).  
- Use `/data` for all persistent state.

---

## 11. Acceptance Criteria

- Config via YAML, secrets via env.  
- Each category produces raw/resolved JSON + PMM YAML.  
- Deduplication works (no repeats <60 days).  
- SQLite DB updated correctly.  
- Local API returns valid data.  
- Full run (3–4 categories, 20 recs each) finishes <2m with warm caches.

---

## 12. Future Extensions

- Feedback system (👍/👎)
- Complex filters (“cozy sci-fi under 100m”)
- Web UI
- Embeddings-based similarity
- Multi-user support

---

## 13. Deliverables

Claude must implement:
- Go project skeleton  
- All packages and config parsing  
- SQLite persistence  
- Job orchestration and scheduler  
- Local HTTP API  
- Working Dockerfile and Makefile  
- Example configs under `/config`  
- Example JSON/YAML outputs under `/data` and `/output`  

---
