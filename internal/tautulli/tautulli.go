package tautulli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/dppeppel/scryarr/internal/logging"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	log = logging.GetLogger("tautulli")
}

// Client handles Tautulli API interactions
type Client struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewClient creates a new Tautulli client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// HistoryItem represents a single watch history entry
type HistoryItem struct {
	Title      string `json:"title"`
	Year       int    `json:"year"`
	MediaType  string `json:"media_type"` // movie, episode
	WatchedAt  int64  `json:"stopped"`
	TMDbID     string `json:"guid"`       // Plex GUID, may need parsing
	Rating     int    `json:"rating_key"`
	ParentTitle string `json:"grandparent_title"` // For TV shows
}

// GetHistory fetches watch history from Tautulli
func (c *Client) GetHistory(lookbackDays int) ([]HistoryItem, error) {
	log.Info().Int("lookback_days", lookbackDays).Msg("fetching watch history from Tautulli")

	params := url.Values{}
	params.Set("apikey", c.apiKey)
	params.Set("cmd", "get_history")
	params.Set("length", "1000") // Max items to fetch
	params.Set("order_column", "date")
	params.Set("order_dir", "desc")

	// Calculate start date
	startDate := time.Now().AddDate(0, 0, -lookbackDays).Unix()
	params.Set("start_date", fmt.Sprintf("%d", startDate))

	reqURL := fmt.Sprintf("%s/api/v2?%s", c.baseURL, params.Encode())

	resp, err := c.client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch history: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tautulli returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response struct {
			Result string `json:"result"`
			Data   struct {
				Data []HistoryItem `json:"data"`
			} `json:"data"`
		} `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Response.Result != "success" {
		return nil, fmt.Errorf("tautulli API returned non-success result")
	}

	log.Info().Int("count", len(result.Response.Data.Data)).Msg("fetched watch history")
	return result.Response.Data.Data, nil
}

// GetMetadata fetches detailed metadata for a specific rating key
func (c *Client) GetMetadata(ratingKey int) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("apikey", c.apiKey)
	params.Set("cmd", "get_metadata")
	params.Set("rating_key", fmt.Sprintf("%d", ratingKey))

	reqURL := fmt.Sprintf("%s/api/v2?%s", c.baseURL, params.Encode())

	resp, err := c.client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tautulli returned status %d", resp.StatusCode)
	}

	var result struct {
		Response struct {
			Result string                 `json:"result"`
			Data   map[string]interface{} `json:"data"`
		} `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Response.Result != "success" {
		return nil, fmt.Errorf("tautulli API returned non-success result")
	}

	return result.Response.Data, nil
}
