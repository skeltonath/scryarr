# Scryarr

**AI-powered movie and TV recommendations for your Plex library**

Scryarr is a local Go service that intelligently recommends movies and TV shows based on your Plex watch history. It integrates with Tautulli, TMDb, and uses an LLM to generate personalized recommendations that are automatically published as Plex Meta Manager (PMM) collections.

---

## Features

- 🎬 **Intelligent Recommendations** - Uses LLMs to generate contextual recommendations based on your watch history
- 📊 **TMDb Integration** - Rich metadata enrichment with genres, keywords, and ratings
- 🎯 **Category-Based** - Support for genre, mood, and seed-based recommendations
- 🔄 **Deduplication** - Prevents duplicate recommendations and filters out titles already in your library
- 📅 **Scheduled Runs** - Automatic daily recommendation updates via cron scheduling
- 🐳 **Dockerized** - Easy deployment on Unraid or any Docker environment
- 🌐 **REST API** - Local HTTP API for integration with future UIs
- 💾 **SQLite Storage** - Lightweight persistence with full job history tracking

---

## Quick Start

### 1. Clone and Configure

```bash
git clone https://github.com/dppeppel/scryarr.git
cd scryarr

# Copy example environment file
cp .env.example .env

# Edit .env with your API keys
nano .env
```

### 2. Configure Categories

Edit `config/categories.yml` to define your recommendation categories. Examples:

- Genre-based (e.g., "True Crime")
- Mood-based (e.g., "Cozy", "Feel Good")
- Seed-based (e.g., "Because you watched The Jinx")

### 3. Run with Docker

```bash
# Build the image
make docker-build

# Run the container
make docker-run
```

Or use Docker Compose:

```yaml
version: '3.8'
services:
  scryarr:
    image: scryarr:latest
    container_name: scryarr
    volumes:
      - ./config:/config
      - ./data:/data
      - ./output:/output
    environment:
      - TAUTULLI_API_KEY=${TAUTULLI_API_KEY}
      - PLEX_TOKEN=${PLEX_TOKEN}
      - TMDB_API_KEY=${TMDB_API_KEY}
      - LLM_API_BASE=${LLM_API_BASE}
      - LLM_API_KEY=${LLM_API_KEY}
    ports:
      - "8080:8080"
    restart: unless-stopped
```

---

## Configuration

### App Configuration (`config/app.yml`)

```yaml
app:
  mode: loop                 # oneshot | loop
  schedule_cron: "0 3 * * *" # Daily at 3 AM
  log_level: info

paths:
  db_path: /data/scryarr.sqlite
  json_out_dir: /data/recommendations
  pmm_out_dir: /output

tautulli:
  url: "http://tautulli:8181"
  lookback_days: 120

recommender:
  model: "gpt-4o-mini"
  recs_per_category: 20
  diversity_min_fraction: 0.3
  recency_weight: 0.6

api:
  enabled: true
  bind_addr: "0.0.0.0:8080"
```

### Categories Configuration (`config/categories.yml`)

Define custom recommendation categories:

```yaml
categories:
  - label: "True Crime"
    type: "genre"
    media_types: ["movie", "tv"]
    tmdb_filters:
      include_genres: ["Documentary", "Crime"]
    keywords_prefer: ["true crime", "investigative journalism"]

  - label: "Cozy"
    type: "keyword"
    media_types: ["movie", "tv"]
    mood_keywords: ["cozy", "gentle", "uplifting"]
```

---

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/health` | GET | Health check |
| `/v1/categories` | GET | List configured categories |
| `/v1/runs/latest` | GET | Latest job run with statuses |
| `/v1/recs/{label}/latest` | GET | Latest resolved recommendations for a category |
| `/v1/recs/{label}/latest/raw` | GET | Raw LLM output for a category |
| `/v1/pmm/collections` | GET | List generated PMM YAML files |
| `/v1/run` | POST | Manually trigger a job run |

---

## Output

Scryarr generates three types of output for each category:

1. **Raw LLM JSON** (`/data/recommendations/raw_*.json`) - Original LLM response
2. **Resolved JSON** (`/data/recommendations/resolved_*.json`) - TMDb-enriched recommendations
3. **PMM YAML** (`/output/recommended__*.yml`) - Plex Meta Manager collection files

PMM collections are named like: `!01_Recommended — True Crime (Movies)`

---

## Development

```bash
# Install dependencies
go mod download

# Format code
make fmt

# Build binary
make build

# Run locally
make run

# Run tests
make test

# Clean artifacts
make clean
```

---

## Architecture

```
scryarr/
├── cmd/worker/          # Main entry point
├── internal/
│   ├── api/            # HTTP API server
│   ├── config/         # Configuration loading
│   ├── llm/            # LLM client (OpenAI-compatible)
│   ├── logging/        # Structured logging (zerolog)
│   ├── plex/           # Plex API client
│   ├── publish/        # JSON and YAML output
│   ├── resolve/        # TMDb resolution and enrichment
│   ├── store/          # SQLite database layer
│   ├── tautulli/       # Tautulli API client
│   └── tmdb/           # TMDb API client
├── config/             # Configuration files
├── data/               # SQLite DB and JSON outputs
└── output/             # PMM YAML collections
```

---

## Requirements

- Docker or Go 1.22+
- Plex Media Server
- Tautulli
- TMDb API Key
- OpenAI-compatible LLM API (OpenAI, OpenRouter, etc.)
- Plex Meta Manager (optional, for automated collection sync)

---

## License

MIT License - See LICENSE file for details

---

## Acknowledgments

Built with:
- [gorilla/mux](https://github.com/gorilla/mux) - HTTP routing
- [zerolog](https://github.com/rs/zerolog) - Structured logging
- [go-openai](https://github.com/sashabaranov/go-openai) - LLM client
- [golang-tmdb](https://github.com/cyruzin/golang-tmdb) - TMDb API
- [cron](https://github.com/robfig/cron) - Job scheduling

---

For questions or issues, please open an issue on GitHub.
