package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dppeppel/scryarr/internal/config"
	"github.com/dppeppel/scryarr/internal/logging"
	"github.com/rs/zerolog"
	openai "github.com/sashabaranov/go-openai"
)

var log zerolog.Logger

func init() {
	log = logging.GetLogger("llm")
}

// Client handles LLM API interactions
type Client struct {
	client *openai.Client
	model  string
}

// NewClient creates a new LLM client
func NewClient(cfg *config.LLMConfig, model string) *Client {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if cfg.APIBase != "" {
		clientConfig.BaseURL = cfg.APIBase
	}

	return &Client{
		client: openai.NewClientWithConfig(clientConfig),
		model:  model,
	}
}

// PromptRequest represents the structured request to the LLM
type PromptRequest struct {
	Task            string                 `json:"task"`
	Category        map[string]interface{} `json:"category"`
	Constraints     map[string]interface{} `json:"constraints"`
	TasteProfile    map[string]interface{} `json:"taste_profile"`
	AlreadySeen     []string               `json:"already_seen"`
	AlreadyRecommended []string            `json:"already_recommended"`
	OutputSchema    map[string]interface{} `json:"output_schema"`
}

// Recommendation represents a single recommendation from the LLM
type Recommendation struct {
	Title    string   `json:"title"`
	Year     int      `json:"year"`
	Medium   string   `json:"medium"` // movie or tv
	Why      string   `json:"why"`
	Keywords []string `json:"keywords"`
}

// LLMResponse represents the response from the LLM
type LLMResponse struct {
	Category        string           `json:"category"`
	GeneratedAt     string           `json:"generated_at"`
	Recommendations []Recommendation `json:"recommendations"`
}

// GenerateRecommendations sends a prompt to the LLM and returns recommendations
func (c *Client) GenerateRecommendations(category *config.Category, constraints map[string]interface{}, tasteProfile, alreadySeen, alreadyRecommended []string) (*LLMResponse, error) {
	log.Info().Str("category", category.Label).Msg("generating recommendations via LLM")

	// Build the prompt request
	req := PromptRequest{
		Task: "recommend",
		Category: map[string]interface{}{
			"label":       category.Label,
			"type":        category.Type,
			"media_types": category.MediaTypes,
		},
		Constraints: constraints,
		TasteProfile: map[string]interface{}{
			"recent_watches": tasteProfile,
		},
		AlreadySeen:        alreadySeen,
		AlreadyRecommended: alreadyRecommended,
		OutputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"category":    map[string]string{"type": "string"},
				"generated_at": map[string]string{"type": "string"},
				"recommendations": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"title":    map[string]string{"type": "string"},
							"year":     map[string]string{"type": "integer"},
							"medium":   map[string]string{"type": "string"},
							"why":      map[string]string{"type": "string"},
							"keywords": map[string]interface{}{
								"type": "array",
								"items": map[string]string{"type": "string"},
							},
						},
					},
				},
			},
		},
	}

	// Add category-specific filters
	if category.TMDbFilters != nil {
		req.Category["tmdb_filters"] = map[string]interface{}{
			"include_genres": category.TMDbFilters.IncludeGenres,
			"exclude_genres": category.TMDbFilters.ExcludeGenres,
		}
	}
	if len(category.KeywordsPrefer) > 0 {
		req.Category["keywords_prefer"] = category.KeywordsPrefer
	}
	if len(category.KeywordsAvoid) > 0 {
		req.Category["keywords_avoid"] = category.KeywordsAvoid
	}
	if len(category.MoodKeywords) > 0 {
		req.Category["mood_keywords"] = category.MoodKeywords
	}
	if category.Seed != nil {
		req.Category["seed"] = category.Seed
	}
	if len(category.Seeds) > 0 {
		req.Category["seeds"] = category.Seeds
	}

	// Convert to JSON
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create OpenAI chat completion request
	systemMsg := "You are a recommender for a private media server. Suggest items constrained by the provided category and constraints. Return strict JSON matching the schema. Do not include already_seen or already_recommended titles. No streaming or acquisition info."

	chatReq := openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemMsg,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: string(reqJSON),
			},
		},
		Temperature: 0.7,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("LLM API request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("LLM returned no choices")
	}

	content := resp.Choices[0].Message.Content

	// Parse the response
	var llmResp LLMResponse
	if err := json.Unmarshal([]byte(content), &llmResp); err != nil {
		log.Error().Err(err).Str("content", content).Msg("failed to parse LLM response")
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	log.Info().Str("category", category.Label).Int("count", len(llmResp.Recommendations)).Msg("generated recommendations")

	// Set generated_at if not set
	if llmResp.GeneratedAt == "" {
		llmResp.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}

	return &llmResp, nil
}
