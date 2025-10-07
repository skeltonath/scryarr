package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

// AppConfig represents the main application configuration from app.yml
type AppConfig struct {
	App          AppSettings          `yaml:"app"`
	Paths        PathSettings         `yaml:"paths"`
	Tautulli     TautulliSettings     `yaml:"tautulli"`
	Plex         PlexSettings         `yaml:"plex"`
	Recommender  RecommenderSettings  `yaml:"recommender"`
	Overseerr    OverseerrSettings    `yaml:"overseerr"`
	API          APISettings          `yaml:"api"`
}

type AppSettings struct {
	Mode         string `yaml:"mode"`           // oneshot | loop
	ScheduleCron string `yaml:"schedule_cron"`  // cron schedule for loop mode
	LogLevel     string `yaml:"log_level"`      // info, debug, warn, error
}

type PathSettings struct {
	DBPath      string `yaml:"db_path"`
	JSONOutDir  string `yaml:"json_out_dir"`
	PMMOutDir   string `yaml:"pmm_out_dir"`
}

type TautulliSettings struct {
	URL          string `yaml:"url"`
	APIKey       string `yaml:"-"` // loaded from env
	LookbackDays int    `yaml:"lookback_days"`
}

type PlexSettings struct {
	URL   string `yaml:"url"`
	Token string `yaml:"-"` // loaded from env
}

type RecommenderSettings struct {
	Model               string   `yaml:"model"`
	RecsPerCategory     int      `yaml:"recs_per_category"`
	DiversityMinFrac    float64  `yaml:"diversity_min_fraction"`
	RecencyWeight       float64  `yaml:"recency_weight"`
	AllowMediaTypes     []string `yaml:"allow_media_types"`
}

type OverseerrSettings struct {
	Enabled             bool   `yaml:"enabled"`
	URL                 string `yaml:"url"`
	APIKey              string `yaml:"-"` // loaded from env
	RequestsPerCategory int    `yaml:"requests_per_category"`
}

type APISettings struct {
	Enabled  bool   `yaml:"enabled"`
	BindAddr string `yaml:"bind_addr"`
}

// CategoriesConfig represents the categories.yml configuration
type CategoriesConfig struct {
	Categories []Category `yaml:"categories"`
}

type Category struct {
	Label         string            `yaml:"label"`
	Type          string            `yaml:"type"` // genre, title_seed, keyword, seed_list
	MediaTypes    []string          `yaml:"media_types"`
	TMDbFilters   *TMDbFilters      `yaml:"tmdb_filters,omitempty"`
	KeywordsPrefer []string         `yaml:"keywords_prefer,omitempty"`
	KeywordsAvoid  []string         `yaml:"keywords_avoid,omitempty"`
	MoodKeywords   []string         `yaml:"mood_keywords,omitempty"`
	Seed          *TitleSeed        `yaml:"seed,omitempty"`
	Seeds         []TitleSeed       `yaml:"seeds,omitempty"`
}

type TMDbFilters struct {
	IncludeGenres []string `yaml:"include_genres,omitempty"`
	ExcludeGenres []string `yaml:"exclude_genres,omitempty"`
}

type TitleSeed struct {
	Title  string `yaml:"title"`
	Year   int    `yaml:"year"`
	Medium string `yaml:"medium"` // movie | tv
}

// LoadAppConfig loads the app.yml configuration file
func LoadAppConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read app config: %w", err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse app config: %w", err)
	}

	// Load secrets from environment variables
	cfg.Tautulli.APIKey = os.Getenv("TAUTULLI_API_KEY")
	cfg.Plex.Token = os.Getenv("PLEX_TOKEN")
	cfg.Overseerr.APIKey = os.Getenv("OVERSEERR_API_KEY")

	return &cfg, nil
}

// LoadCategoriesConfig loads the categories.yml configuration file
func LoadCategoriesConfig(path string) (*CategoriesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read categories config: %w", err)
	}

	var cfg CategoriesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse categories config: %w", err)
	}

	return &cfg, nil
}

// LLMConfig holds LLM-specific configuration loaded from env
type LLMConfig struct {
	APIBase string
	APIKey  string
}

// LoadLLMConfig loads LLM configuration from environment variables
func LoadLLMConfig() *LLMConfig {
	return &LLMConfig{
		APIBase: os.Getenv("LLM_API_BASE"),
		APIKey:  os.Getenv("LLM_API_KEY"),
	}
}

// TMDbConfig holds TMDb-specific configuration loaded from env
type TMDbConfig struct {
	APIKey string
}

// LoadTMDbConfig loads TMDb configuration from environment variables
func LoadTMDbConfig() *TMDbConfig {
	return &TMDbConfig{
		APIKey: os.Getenv("TMDB_API_KEY"),
	}
}
