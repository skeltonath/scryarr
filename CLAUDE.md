# CLAUDE.md

## 👋 Welcome

Hello Claude — you are a **developer on an open-source personal project** for a media server called **Scryarr**.  
Scryarr is part of the *arr-style ecosystem* (e.g., Sonarr, Radarr, Prowlarr, Grabarr) and is designed to integrate deeply with Plex, Tautulli, TMDb, and Plex Meta Manager (PMM).

Your role is to help develop, maintain, and improve this project — writing clean, idiomatic Go code and contributing to a stable, extensible service.

---

## 🧭 Overview of Scryarr

**Scryarr** is a local Go service that intelligently recommends movies and TV shows based on a user's Plex watch history.

It uses:
- **Tautulli** to track Plex watch data
- **TMDb / TVDb** to enrich metadata
- **An LLM** (via an OpenAI-compatible API) to generate smart recommendations
- **Plex Meta Manager** to publish those recommendations as collections

Scryarr runs entirely **on-prem**, stores state in SQLite, and exposes a **local API** for future UI or automation.

---

## 🧩 Key Concepts

- **Category** – the base concept for generating recommendations.  
  Each category may represent a genre, a specific title seed (“Because you watched…”), or an abstract mood (“cozy”, “bleak”, etc.).
- **Job Run** – a full cycle of fetching watch history, generating recs, resolving metadata, and publishing output.
- **LLM Prompt** – a structured JSON query that defines what to generate.
- **Resolved Output** – TMDb-enriched recommendation data saved as JSON.
- **PMM Collections** – Plex Meta Manager YAML files created for each category.

---

## ⚙️ Major Technologies Used

- **Language:** Go 1.22+
- **Database:** SQLite (via `mattn/go-sqlite3`)
- **Scheduler:** `robfig/cron/v3`
- **Router:** `gorilla/mux`
- **YAML Parser:** `goccy/go-yaml`
- **Logging:** `rs/zerolog`
- **LLM Client:** `sashabaranov/go-openai` (OpenAI-compatible API)
- **Metadata API:** `cyruzin/golang-tmdb`

---

## 📦 Major Dependencies

| Purpose | Library |
|----------|----------|
| Router | `github.com/gorilla/mux` |
| YAML | `github.com/goccy/go-yaml` |
| SQLite | `github.com/mattn/go-sqlite3` |
| Logging | `github.com/rs/zerolog` |
| Cron | `github.com/robfig/cron/v3` |
| LLM | `github.com/sashabaranov/go-openai` |
| TMDb | `github.com/cyruzin/golang-tmdb` |

---

## 🔗 API Structure

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v1/health` | Service health check |
| `GET` | `/v1/categories` | List configured categories |
| `GET` | `/v1/runs/latest` | Latest job run with statuses |
| `GET` | `/v1/recs/{label}/latest` | Latest resolved recs for a category |
| `GET` | `/v1/recs/{label}/latest/raw` | Raw LLM output for a category |
| `GET` | `/v1/pmm/collections` | List generated PMM YAML files |
| `POST` | `/v1/run` | Trigger a full recommendation cycle manually |

All endpoints are **local-only**. No auth, no CORS.

---

## 🗂️ File Structure

```
scryarr/
  cmd/
    worker/main.go
  internal/
    api/
    tautulli/
    plex/
    tmdb/
    llm/
    resolve/
    publish/
    store/
  config/
    app.yml
    categories.yml
  data/
    scryarr.sqlite
    recommendations/
  output/
    recommended__*.yml
  Dockerfile
  go.mod
  CLAUDE.md
```

---

## 🧪 Development Workflow

Follow this workflow for each new feature or bugfix:

1. **Create a new branch**  
   Name it `feature/<name>` or `fix/<name>`.

2. **Implement**  
   Add or update the feature, maintaining modular Go code and clean package boundaries.

3. **Add/Update Unit Tests**  
   Write tests for any new logic or edge cases.

4. **Run Tests**  
   Ensure all tests pass locally:
   ```bash
   go test ./...
   ```

5. **Commit and Push**  
   Use clear, conventional commit messages.

6. **Squash Merge**  
   When the feature is verified, squash merge it back into `main`.

---

## 🧰 How to Test Scryarr

*(To be expanded as test suite develops.)*  
Include instructions for running local unit and integration tests, mocking API calls, and verifying PMM YAML output.

---

## 💻 Useful Commands

```bash
# Run service in oneshot mode
go run ./cmd/worker --config ./config/app.yml

# Build Docker image
docker build -t scryarr .

# Run container locally
docker run -v $(pwd)/config:/config -v $(pwd)/data:/data -v $(pwd)/output:/output scryarr

# Format and tidy
go fmt ./...
go mod tidy
```

---

## 🧠 Things to Remember

- Scryarr is **local-first** — never send personal data or Plex metadata externally.  
- Keep **LLM prompts** deterministic and machine-readable (strict JSON).  
- Maintain **consistent TMDb ID resolution** for PMM YAMLs.  
- Avoid redundant DB writes; always use prepared statements.  
- Keep code and architecture simple — this is a foundation for future UI and automation.  
- Be modular and future-proof: Scryarr may later support categories like moods, titles, or directors.

---
