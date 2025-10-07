package resolve

import (
	"fmt"
	"strings"
	"time"

	"github.com/dppeppel/scryarr/internal/llm"
	"github.com/dppeppel/scryarr/internal/logging"
	"github.com/dppeppel/scryarr/internal/store"
	"github.com/dppeppel/scryarr/internal/tmdb"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	log = logging.GetLogger("resolve")
}

// ResolvedItem represents a fully resolved recommendation with TMDb metadata
type ResolvedItem struct {
	Title      string   `json:"title"`
	Year       int      `json:"year"`
	Medium     string   `json:"medium"` // movie or tv
	TMDbID     int      `json:"tmdb_id"`
	IMDbID     string   `json:"imdb_id,omitempty"`
	RuntimeMin int      `json:"runtime_min,omitempty"`
	VoteCount  int      `json:"vote_count,omitempty"`
	VoteAvg    float64  `json:"vote_avg,omitempty"`
	Why        string   `json:"why"`
	Keywords   []string `json:"keywords"`
	Genres     []string `json:"genres,omitempty"`
}

// ResolvedOutput represents the final resolved recommendations for a category
type ResolvedOutput struct {
	Category   string         `json:"category"`
	ResolvedAt string         `json:"resolved_at"`
	Items      []ResolvedItem `json:"items"`
}

// Resolver handles resolution of LLM recommendations to TMDb metadata
type Resolver struct {
	tmdbClient *tmdb.Client
	store      *store.Store
}

// NewResolver creates a new resolver
func NewResolver(tmdbClient *tmdb.Client, store *store.Store) *Resolver {
	return &Resolver{
		tmdbClient: tmdbClient,
		store:      store,
	}
}

// Resolve takes LLM recommendations and resolves them to TMDb IDs with full metadata
func (r *Resolver) Resolve(llmResp *llm.LLMResponse, categoryLabel string) (*ResolvedOutput, error) {
	log.Info().Str("category", categoryLabel).Int("count", len(llmResp.Recommendations)).Msg("resolving recommendations")

	var resolved []ResolvedItem

	// Get already recommended items for deduplication (last 60 days)
	since := time.Now().AddDate(0, 0, -60)
	alreadyRecommended, err := r.store.GetRecommendationsSince(categoryLabel, since)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get recommendation history")
		alreadyRecommended = make(map[int]bool)
	}

	for _, rec := range llmResp.Recommendations {
		// Normalize media type (handle various formats from LLM)
		mediaType := strings.ToLower(rec.Medium)
		if mediaType == "show" || mediaType == "series" {
			mediaType = "tv"
		}

		// Search TMDb
		result, err := r.tmdbClient.SearchAndResolve(rec.Title, rec.Year, mediaType)
		if err != nil {
			log.Warn().Err(err).Str("title", rec.Title).Int("year", rec.Year).Msg("failed to resolve title")
			continue
		}

		// Check if already recommended
		if alreadyRecommended[result.TMDbID] {
			log.Debug().Str("title", result.Title).Int("tmdb_id", result.TMDbID).Msg("skipping duplicate")
			continue
		}

		// Check if in Plex inventory by TMDb ID
		inPlex, err := r.store.IsInPlexInventory(result.TMDbID, mediaType)
		if err != nil {
			log.Warn().Err(err).Msg("failed to check Plex inventory")
		}
		if inPlex {
			log.Debug().Str("title", result.Title).Int("tmdb_id", result.TMDbID).Msg("skipping item already in Plex")
			continue
		}

		// Add to resolved list
		item := ResolvedItem{
			Title:      result.Title,
			Year:       result.Year,
			Medium:     mediaType,
			TMDbID:     result.TMDbID,
			IMDbID:     result.IMDbID,
			RuntimeMin: result.RuntimeMin,
			VoteCount:  result.VoteCount,
			VoteAvg:    result.VoteAvg,
			Why:        rec.Why,
			Keywords:   rec.Keywords,
			Genres:     result.Genres,
		}

		resolved = append(resolved, item)

		// Record in history
		if err := r.store.RecordRecommendation(categoryLabel, result.TMDbID, mediaType); err != nil {
			log.Warn().Err(err).Msg("failed to record recommendation")
		}

		// Mark as seen to prevent duplicates in this batch
		alreadyRecommended[result.TMDbID] = true
	}

	if len(resolved) == 0 {
		return nil, fmt.Errorf("no recommendations could be resolved")
	}

	output := &ResolvedOutput{
		Category:   categoryLabel,
		ResolvedAt: time.Now().UTC().Format(time.RFC3339),
		Items:      resolved,
	}

	log.Info().Str("category", categoryLabel).Int("resolved", len(resolved)).Msg("resolution complete")

	return output, nil
}
