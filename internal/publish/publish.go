package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dppeppel/scryarr/internal/llm"
	"github.com/dppeppel/scryarr/internal/logging"
	"github.com/dppeppel/scryarr/internal/resolve"
	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	log = logging.GetLogger("publish")
}

// Publisher handles output of JSON files and PMM YAML files
type Publisher struct {
	jsonOutDir string
	pmmOutDir  string
}

// NewPublisher creates a new publisher
func NewPublisher(jsonOutDir, pmmOutDir string) *Publisher {
	return &Publisher{
		jsonOutDir: jsonOutDir,
		pmmOutDir:  pmmOutDir,
	}
}

// PublishResult contains the paths to published files
type PublishResult struct {
	RawJSONPath      string
	ResolvedJSONPath string
	PMMMovieYAMLPath string
	PMMTVYAMLPath    string
}

// Publish writes both raw LLM output and resolved recommendations, then generates PMM YAMLs
func (p *Publisher) Publish(categoryLabel string, llmResp *llm.LLMResponse, resolved *resolve.ResolvedOutput) (*PublishResult, error) {
	log.Info().Str("category", categoryLabel).Msg("publishing outputs")

	result := &PublishResult{}

	// Write raw LLM JSON
	rawPath, err := p.writeRawJSON(categoryLabel, llmResp)
	if err != nil {
		return nil, fmt.Errorf("failed to write raw JSON: %w", err)
	}
	result.RawJSONPath = rawPath

	// Write resolved JSON
	resolvedPath, err := p.writeResolvedJSON(categoryLabel, resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to write resolved JSON: %w", err)
	}
	result.ResolvedJSONPath = resolvedPath

	// Generate PMM YAMLs (separate for movies and TV)
	moviePath, tvPath, err := p.generatePMMYAMLs(categoryLabel, resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PMM YAMLs: %w", err)
	}
	result.PMMMovieYAMLPath = moviePath
	result.PMMTVYAMLPath = tvPath

	log.Info().Str("category", categoryLabel).Msg("publish complete")

	return result, nil
}

func (p *Publisher) writeRawJSON(categoryLabel string, llmResp *llm.LLMResponse) (string, error) {
	filename := fmt.Sprintf("raw_%s.json", sanitizeFilename(categoryLabel))
	path := filepath.Join(p.jsonOutDir, filename)

	data, err := json.MarshalIndent(llmResp, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	log.Debug().Str("path", path).Msg("wrote raw JSON")
	return path, nil
}

func (p *Publisher) writeResolvedJSON(categoryLabel string, resolved *resolve.ResolvedOutput) (string, error) {
	filename := fmt.Sprintf("resolved_%s.json", sanitizeFilename(categoryLabel))
	path := filepath.Join(p.jsonOutDir, filename)

	data, err := json.MarshalIndent(resolved, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	log.Debug().Str("path", path).Msg("wrote resolved JSON")
	return path, nil
}

func (p *Publisher) generatePMMYAMLs(categoryLabel string, resolved *resolve.ResolvedOutput) (string, string, error) {
	var movies []int
	var tvShows []int

	for _, item := range resolved.Items {
		if item.Medium == "movie" {
			movies = append(movies, item.TMDbID)
		} else if item.Medium == "tv" {
			tvShows = append(tvShows, item.TMDbID)
		}
	}

	var moviePath, tvPath string

	// Generate movie collection YAML
	if len(movies) > 0 {
		path, err := p.writePMMYAML(categoryLabel, "Movies", movies, "tmdb_movie")
		if err != nil {
			return "", "", err
		}
		moviePath = path
	}

	// Generate TV collection YAML
	if len(tvShows) > 0 {
		path, err := p.writePMMYAML(categoryLabel, "Series", tvShows, "tmdb_show")
		if err != nil {
			return "", "", err
		}
		tvPath = path
	}

	return moviePath, tvPath, nil
}

func (p *Publisher) writePMMYAML(categoryLabel, mediaType string, ids []int, tmdbKey string) (string, error) {
	collectionName := fmt.Sprintf("!01_Recommended — %s (%s)", categoryLabel, mediaType)

	collection := map[string]interface{}{
		tmdbKey:     ids,
		"sync_mode": "replace",
		"summary":   "AI-curated picks based on Plex history & category prefs.",
	}

	yamlData := map[string]interface{}{
		"collections": map[string]interface{}{
			collectionName: collection,
		},
	}

	data, err := yaml.Marshal(yamlData)
	if err != nil {
		return "", err
	}

	filename := fmt.Sprintf("recommended__%s__%s.yml",
		strings.ToLower(mediaType),
		sanitizeFilename(categoryLabel))
	path := filepath.Join(p.pmmOutDir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	log.Debug().Str("path", path).Int("count", len(ids)).Msg("wrote PMM YAML")
	return path, nil
}

func sanitizeFilename(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "—", "-")
	s = strings.ReplaceAll(s, "–", "-")

	// Remove any non-alphanumeric characters except dash and underscore
	var result []rune
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result = append(result, r)
		}
	}

	return string(result)
}
