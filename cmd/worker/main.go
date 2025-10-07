package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/dppeppel/scryarr/internal/api"
	"github.com/dppeppel/scryarr/internal/config"
	"github.com/dppeppel/scryarr/internal/llm"
	"github.com/dppeppel/scryarr/internal/logging"
	"github.com/dppeppel/scryarr/internal/plex"
	"github.com/dppeppel/scryarr/internal/publish"
	"github.com/dppeppel/scryarr/internal/resolve"
	"github.com/dppeppel/scryarr/internal/store"
	"github.com/dppeppel/scryarr/internal/tautulli"
	"github.com/dppeppel/scryarr/internal/tmdb"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

var (
	configPath       = flag.String("config", "/config/app.yml", "Path to app.yml config file")
	categoriesPath   = flag.String("categories", "/config/categories.yml", "Path to categories.yml config file")
)

func main() {
	flag.Parse()

	// Load configuration
	appCfg, err := config.LoadAppConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load app config: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	logging.Setup(appCfg.App.LogLevel)
	log.Info().Msg("Starting Scryarr worker")

	// Load categories
	categoriesCfg, err := config.LoadCategoriesConfig(*categoriesPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load categories config")
	}

	// Ensure output directories exist
	if err := os.MkdirAll(appCfg.Paths.JSONOutDir, 0755); err != nil {
		log.Fatal().Err(err).Msg("Failed to create JSON output directory")
	}
	if err := os.MkdirAll(appCfg.Paths.PMMOutDir, 0755); err != nil {
		log.Fatal().Err(err).Msg("Failed to create PMM output directory")
	}

	// Initialize store
	db, err := store.NewStore(appCfg.Paths.DBPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer db.Close()

	// Create orchestrator
	orch := NewOrchestrator(appCfg, categoriesCfg, db)

	// Start API server if enabled
	var apiServer *api.Server
	if appCfg.API.Enabled {
		apiServer = api.NewServer(
			db,
			categoriesCfg,
			appCfg.Paths.JSONOutDir,
			appCfg.Paths.PMMOutDir,
			appCfg.API.BindAddr,
			func() error { return orch.Run() },
		)

		go func() {
			if err := apiServer.Start(); err != nil {
				log.Fatal().Err(err).Msg("API server failed")
			}
		}()
	}

	// Run based on mode
	if appCfg.App.Mode == "oneshot" {
		log.Info().Msg("Running in oneshot mode")
		if err := orch.Run(); err != nil {
			log.Error().Err(err).Msg("Job run failed")
			os.Exit(1)
		}
		log.Info().Msg("Oneshot complete")
	} else if appCfg.App.Mode == "loop" {
		log.Info().Str("schedule", appCfg.App.ScheduleCron).Msg("Running in loop mode")

		c := cron.New()
		_, err := c.AddFunc(appCfg.App.ScheduleCron, func() {
			log.Info().Msg("Scheduled job starting")
			if err := orch.Run(); err != nil {
				log.Error().Err(err).Msg("Scheduled job failed")
			}
		})
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to schedule cron job")
		}

		c.Start()

		// Wait for interrupt signal
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig

		log.Info().Msg("Shutting down")
		c.Stop()
	} else {
		log.Fatal().Str("mode", appCfg.App.Mode).Msg("Unknown mode (must be 'oneshot' or 'loop')")
	}
}

// Orchestrator coordinates the full recommendation workflow
type Orchestrator struct {
	appCfg        *config.AppConfig
	categoriesCfg *config.CategoriesConfig
	store         *store.Store
	mu            sync.Mutex // Prevent concurrent runs
}

// NewOrchestrator creates a new orchestrator
func NewOrchestrator(appCfg *config.AppConfig, categoriesCfg *config.CategoriesConfig, store *store.Store) *Orchestrator {
	return &Orchestrator{
		appCfg:        appCfg,
		categoriesCfg: categoriesCfg,
		store:         store,
	}
}

// Run executes a full recommendation cycle
func (o *Orchestrator) Run() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	log.Info().Msg("Starting job run")

	// Create job run record
	jobID, err := o.store.CreateJobRun(o.appCfg.App.Mode)
	if err != nil {
		return fmt.Errorf("failed to create job run: %w", err)
	}

	// Initialize clients
	tautulliClient := tautulli.NewClient(o.appCfg.Tautulli.URL, o.appCfg.Tautulli.APIKey)
	plexClient := plex.NewClient(o.appCfg.Plex.URL, o.appCfg.Plex.Token)
	tmdbCfg := config.LoadTMDbConfig()
	tmdbClient, err := tmdb.NewClient(tmdbCfg.APIKey, o.store)
	if err != nil {
		o.store.UpdateJobRun(jobID, "failed", strPtr(err.Error()))
		return fmt.Errorf("failed to create TMDb client: %w", err)
	}
	llmCfg := config.LoadLLMConfig()
	llmClient := llm.NewClient(llmCfg, o.appCfg.Recommender.Model)
	resolver := resolve.NewResolver(tmdbClient, o.store)
	publisher := publish.NewPublisher(o.appCfg.Paths.JSONOutDir, o.appCfg.Paths.PMMOutDir)

	// Fetch Plex inventory
	log.Info().Msg("Fetching Plex inventory")
	inventory, err := plexClient.GetInventory()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch Plex inventory, continuing without it")
	} else {
		var items []struct{ TMDbID int; MediaType string }
		for _, item := range inventory {
			items = append(items, struct{ TMDbID int; MediaType string }{item.TMDbID, item.Type})
		}
		if err := o.store.UpdatePlexInventory(items); err != nil {
			log.Warn().Err(err).Msg("Failed to update Plex inventory in DB")
		}
	}

	// Fetch watch history for taste profile
	log.Info().Msg("Fetching watch history")
	history, err := tautulliClient.GetHistory(o.appCfg.Tautulli.LookbackDays)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch watch history")
		history = []tautulli.HistoryItem{}
	}

	// Build taste profile (titles watched recently)
	var tasteProfile []string
	for _, item := range history {
		if item.ParentTitle != "" {
			tasteProfile = append(tasteProfile, fmt.Sprintf("%s (%d)", item.ParentTitle, item.Year))
		} else {
			tasteProfile = append(tasteProfile, fmt.Sprintf("%s (%d)", item.Title, item.Year))
		}
		if len(tasteProfile) >= 50 {
			break
		}
	}

	// Process each category
	for _, category := range o.categoriesCfg.Categories {
		log.Info().Str("category", category.Label).Msg("Processing category")

		catRunID, err := o.store.CreateCategoryRun(jobID, category.Label, category.Type)
		if err != nil {
			log.Error().Err(err).Str("category", category.Label).Msg("Failed to create category run")
			continue
		}

		if err := o.processCategory(&category, catRunID, llmClient, resolver, publisher, tasteProfile); err != nil {
			log.Error().Err(err).Str("category", category.Label).Msg("Category processing failed")
			o.store.UpdateCategoryRun(catRunID, "failed", nil, strPtr(err.Error()))
			continue
		}
	}

	// Mark job as completed
	if err := o.store.UpdateJobRun(jobID, "completed", nil); err != nil {
		log.Error().Err(err).Msg("Failed to update job run status")
	}

	log.Info().Msg("Job run completed")
	return nil
}

func (o *Orchestrator) processCategory(
	category *config.Category,
	catRunID int64,
	llmClient *llm.Client,
	resolver *resolve.Resolver,
	publisher *publish.Publisher,
	tasteProfile []string,
) error {
	// Build constraints
	constraints := map[string]interface{}{
		"count":                  o.appCfg.Recommender.RecsPerCategory,
		"recency_weight":         o.appCfg.Recommender.RecencyWeight,
		"diversity_min_fraction": o.appCfg.Recommender.DiversityMinFrac,
	}

	// Get already seen (from watch history or Plex inventory)
	var alreadySeen []string
	// TODO: Build from Plex inventory

	// Get already recommended (last 60 days)
	var alreadyRecommended []string
	// TODO: Build from recommendation history

	// Generate recommendations via LLM
	llmResp, err := llmClient.GenerateRecommendations(category, constraints, tasteProfile, alreadySeen, alreadyRecommended)
	if err != nil {
		return fmt.Errorf("LLM generation failed: %w", err)
	}

	// Resolve to TMDb IDs
	resolved, err := resolver.Resolve(llmResp, category.Label)
	if err != nil {
		return fmt.Errorf("resolution failed: %w", err)
	}

	// Publish outputs
	result, err := publisher.Publish(category.Label, llmResp, resolved)
	if err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	// Update category run with paths
	paths := map[string]*string{
		"raw_json":      &result.RawJSONPath,
		"resolved_json": &result.ResolvedJSONPath,
		"pmm_movie":     &result.PMMMovieYAMLPath,
		"pmm_tv":        &result.PMMTVYAMLPath,
	}

	if err := o.store.UpdateCategoryRun(catRunID, "completed", paths, nil); err != nil {
		log.Warn().Err(err).Msg("Failed to update category run")
	}

	return nil
}

func strPtr(s string) *string {
	return &s
}
