# Appendix: Implementation References for PlexRecs

## A. Core Integrations

### 1. Tautulli API
Docs: https://github.com/Tautulli/Tautulli/wiki/Tautulli-API-Reference  
Use `get_history` and `get_metadata`. Authenticate via API key.

### 2. Plex API
Docs: https://support.plex.tv/articles/201638786-plex-media-server-url-commands/  
Use `/library/all` to list inventory. Authenticate with Plex Token.

### 3. TMDb API
Docs: https://developer.themoviedb.org/reference/intro/getting-started  
Endpoints: `/search/movie`, `/search/tv`, `/movie/{id}`, `/tv/{id}`, `/movie/{id}/keywords`.  
Auth via Bearer token.

### 4. TVDb API (optional)
Docs: https://thetvdb.github.io/v4-api/

### 5. Plex Meta Manager (PMM)
Docs: https://metamanager.wiki/en/latest/  
Use YAML syntax matching `tmdb_movie:` and `tmdb_show:` arrays.

### 6. Overseerr API (optional)
Docs: https://api-docs.overseerr.dev/  
Use `POST /api/v1/request` to create requests. Auth via `X-Api-Key`.

---

## B. Supporting Libraries

| Purpose | Library |
|----------|----------|
| Router | github.com/gorilla/mux |
| YAML | github.com/goccy/go-yaml |
| SQLite | github.com/mattn/go-sqlite3 |
| Logging | github.com/rs/zerolog |
| Cron | github.com/robfig/cron/v3 |
| LLM | github.com/sashabaranov/go-openai |
| TMDb | github.com/cyruzin/golang-tmdb |

---

## C. Implementation Hints

- Implement lightweight file-based cache for TMDb lookups.  
- Fallback to TMDb discover if LLM fails.  
- Handle JSON validation errors gracefully.  
- Never expose Plex user data. Only send titles and years externally.  
- Use PRAGMA WAL in SQLite for concurrent API and worker reads.

---

## D. Example Projects

- go-plex-client: https://github.com/jrudio/go-plex-client  
- Plex Meta Manager Configs: https://github.com/meisnate12/Plex-Meta-Manager-Configs  
- Overseerr Go Example: https://github.com/tywil04/go-overseerr  

---

## E. Folder Layout

```
plexrecs/
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
    plexrecs.sqlite
    recommendations/
  output/
    recommended__*.yml
  Dockerfile
  go.mod
```

---

## F. Go Implementation Addendum

Use the following Go libraries and patterns:

- Router: `github.com/gorilla/mux`  
- YAML: `github.com/goccy/go-yaml`  
- Use `zerolog` for structured logs.  
- Use `robfig/cron/v3` for scheduling.  
- Use `mattn/go-sqlite3` for database operations.  
- Implement all YAML and PMM generation with `goccy/go-yaml`.  
- Implement the API with `gorilla/mux`.  

**Example router:**
```go
r := mux.NewRouter()
r.HandleFunc("/v1/health", handleHealth).Methods("GET")
r.HandleFunc("/v1/recs/{label}/latest", handleLatestRecs).Methods("GET")
http.ListenAndServe(cfg.API.BindAddr, r)
```

**Example YAML write:**
```go
out, _ := yaml.Marshal(collections)
os.WriteFile("/output/recommended_truecrime_movies.yml", out, 0644)
```

---
