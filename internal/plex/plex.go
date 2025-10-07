package plex

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dppeppel/scryarr/internal/logging"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	log = logging.GetLogger("plex")
}

// Client handles Plex API interactions
type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewClient creates a new Plex client
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// MediaItem represents a movie or TV show in the Plex library
type MediaItem struct {
	Title     string
	Year      int
	Type      string // movie or show
	TMDbID    int
	IMDbID    string
	RatingKey string
}

// MediaContainer is the XML response structure from Plex
type MediaContainer struct {
	XMLName xml.Name `xml:"MediaContainer"`
	Video   []Video  `xml:"Video"`

	// For TV shows
	Directory []Directory `xml:"Directory"`
}

type Video struct {
	Title       string  `xml:"title,attr"`
	Year        int     `xml:"year,attr"`
	Type        string  `xml:"type,attr"`
	RatingKey   string  `xml:"ratingKey,attr"`
	GUIDAttr    string  `xml:"guid,attr"`    // Old agent format
	GUID        []GUID  `xml:"Guid"`         // New agent format
	Media       []Media `xml:"Media"`
}

type Directory struct {
	Title       string  `xml:"title,attr"`
	Year        int     `xml:"year,attr"`
	Type        string  `xml:"type,attr"`
	RatingKey   string  `xml:"ratingKey,attr"`
	GUIDAttr    string  `xml:"guid,attr"`    // Old agent format
	GUID        []GUID  `xml:"Guid"`         // New agent format
	Media       []Media `xml:"Media"`
}

type GUID struct {
	ID string `xml:"id,attr"`
}

type Media struct {
	Part []Part `xml:"Part"`
}

type Part struct {
	File string `xml:"file,attr"`
}

// GetInventory fetches all movies and TV shows from Plex library
// cachedTMDbIDs is a map of "ratingKey" -> TMDb ID from previous runs to avoid redundant API calls
func (c *Client) GetInventory(cachedTMDbIDs map[string]int) ([]MediaItem, error) {
	log.Info().Msg("fetching Plex library inventory")

	var allItems []MediaItem

	// Fetch movies
	movies, err := c.getLibrarySection("movie", cachedTMDbIDs)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch movies, continuing")
	} else {
		allItems = append(allItems, movies...)
	}

	// Fetch TV shows
	shows, err := c.getLibrarySection("show", cachedTMDbIDs)
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch TV shows, continuing")
	} else {
		allItems = append(allItems, shows...)
	}

	log.Info().Int("count", len(allItems)).Msg("fetched Plex inventory")
	return allItems, nil
}

func (c *Client) getLibrarySection(mediaType string, cachedTMDbIDs map[string]int) ([]MediaItem, error) {
	// Get all library sections
	sections, err := c.getLibrarySections()
	if err != nil {
		return nil, err
	}

	var items []MediaItem
	for _, section := range sections {
		if section.Type == mediaType {
			sectionItems, err := c.getLibrarySectionContents(section.Key, cachedTMDbIDs)
			if err != nil {
				log.Warn().Err(err).Str("section", section.Title).Msg("failed to fetch section")
				continue
			}
			items = append(items, sectionItems...)
		}
	}

	return items, nil
}

type LibrarySection struct {
	Key   string `xml:"key,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
}

func (c *Client) getLibrarySections() ([]LibrarySection, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/library/sections", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", c.token)
	req.Header.Set("Accept", "application/xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch library sections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex returned status %d", resp.StatusCode)
	}

	var container struct {
		XMLName   xml.Name         `xml:"MediaContainer"`
		Directory []LibrarySection `xml:"Directory"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := xml.Unmarshal(body, &container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}


	return container.Directory, nil
}

func (c *Client) getLibrarySectionContents(sectionKey string, cachedTMDbIDs map[string]int) ([]MediaItem, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/library/sections/"+sectionKey+"/all", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", c.token)
	req.Header.Set("Accept", "application/xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch section contents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex returned status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var container MediaContainer
	if err := xml.Unmarshal(body, &container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var items []MediaItem

	// Process videos (movies)
	for _, v := range container.Video {
		item := MediaItem{
			Title:     v.Title,
			Year:      v.Year,
			Type:      "movie",
			RatingKey: v.RatingKey,
		}
		// Parse GUIDs - check both old agent format (guid attr) and new format (Guid children)
		guidStrings := []string{}

		// Check old agent format first (guid attribute)
		if v.GUIDAttr != "" {
			guidStrings = append(guidStrings, v.GUIDAttr)
			if tmdbID := parseTMDbID(v.GUIDAttr); tmdbID > 0 {
				item.TMDbID = tmdbID
			}
			if imdbID := parseIMDbID(v.GUIDAttr); imdbID != "" {
				item.IMDbID = imdbID
			}
		}

		// Check new agent format (Guid children)
		for _, guid := range v.GUID {
			guidStrings = append(guidStrings, guid.ID)
			if tmdbID := parseTMDbID(guid.ID); tmdbID > 0 {
				item.TMDbID = tmdbID
			}
			if imdbID := parseIMDbID(guid.ID); imdbID != "" {
				item.IMDbID = imdbID
			}
		}

		// If no TMDb ID found in GUIDs, try to extract from file path
		if item.TMDbID == 0 {
			for _, media := range v.Media {
				for _, part := range media.Part {
					if tmdbID := extractTMDbIDFromPath(part.File); tmdbID > 0 {
						item.TMDbID = tmdbID
						log.Debug().Str("title", v.Title).Int("tmdb_id", tmdbID).Str("path", part.File).Msg("extracted TMDb ID from file path")
						break
					}
				}
				if item.TMDbID > 0 {
					break
				}
			}
		}

		// Add item even without TMDb ID - we can match by title later
		if item.TMDbID > 0 || len(guidStrings) > 0 {
			items = append(items, item)
		}
	}

	// Process directories (TV shows)
	for _, d := range container.Directory {
		item := MediaItem{
			Title:     d.Title,
			Year:      d.Year,
			Type:      "tv",
			RatingKey: d.RatingKey,
		}
		// Parse GUIDs - check both old agent format (guid attr) and new format (Guid children)
		guidStrings := []string{}

		// Check old agent format first (guid attribute)
		if d.GUIDAttr != "" {
			guidStrings = append(guidStrings, d.GUIDAttr)
			if tmdbID := parseTMDbID(d.GUIDAttr); tmdbID > 0 {
				item.TMDbID = tmdbID
			}
			if imdbID := parseIMDbID(d.GUIDAttr); imdbID != "" {
				item.IMDbID = imdbID
			}
		}

		// Check new agent format (Guid children)
		for _, guid := range d.GUID {
			guidStrings = append(guidStrings, guid.ID)
			if tmdbID := parseTMDbID(guid.ID); tmdbID > 0 {
				item.TMDbID = tmdbID
			}
			if imdbID := parseIMDbID(guid.ID); imdbID != "" {
				item.IMDbID = imdbID
			}
		}

		// If no TMDb ID found in GUIDs, try to extract from file path
		if item.TMDbID == 0 {
			for _, media := range d.Media {
				for _, part := range media.Part {
					if tmdbID := extractTMDbIDFromPath(part.File); tmdbID > 0 {
						item.TMDbID = tmdbID
						log.Debug().Str("title", d.Title).Int("tmdb_id", tmdbID).Str("path", part.File).Msg("extracted TMDb ID from file path")
						break
					}
				}
				if item.TMDbID > 0 {
					break
				}
			}
		}

		// Check cache first before making API call
		if item.TMDbID == 0 {
			if cachedID, ok := cachedTMDbIDs[d.RatingKey]; ok {
				item.TMDbID = cachedID
				log.Debug().Str("title", d.Title).Int("tmdb_id", cachedID).Msg("using cached TMDb ID")
			}
		}

		// If still no TMDb ID, fetch individual metadata (TV shows need this)
		if item.TMDbID == 0 {
			metadata, err := c.getItemMetadata(d.RatingKey)
			if err != nil {
				log.Warn().Err(err).Str("title", d.Title).Str("rating_key", d.RatingKey).Msg("failed to fetch item metadata")
			} else {
				// Check for Guid children in the detailed metadata
				if len(metadata.Directory) > 0 && len(metadata.Directory[0].GUID) > 0 {
					for _, guid := range metadata.Directory[0].GUID {
						if tmdbID := parseTMDbID(guid.ID); tmdbID > 0 {
							item.TMDbID = tmdbID
							log.Debug().Str("title", d.Title).Int("tmdb_id", tmdbID).Msg("extracted TMDb ID from detailed metadata")
							break
						}
					}
				}
			}
		}

		// Add item even without TMDb ID - we can match by title later
		if item.TMDbID > 0 || len(guidStrings) > 0 {
			items = append(items, item)
		}
	}

	return items, nil
}

// parseTMDbID extracts TMDb ID from Plex GUID like "tmdb://12345"
func parseTMDbID(guid string) int {
	if strings.HasPrefix(guid, "tmdb://") {
		var id int
		fmt.Sscanf(guid, "tmdb://%d", &id)
		return id
	}
	return 0
}

// parseIMDbID extracts IMDb ID from Plex GUID like "imdb://tt1234567"
func parseIMDbID(guid string) string {
	if strings.HasPrefix(guid, "imdb://") {
		return strings.TrimPrefix(guid, "imdb://")
	}
	return ""
}

// extractTMDbIDFromPath extracts TMDb ID from file paths like "/path/{tmdb-12345}/file.mkv"
func extractTMDbIDFromPath(filePath string) int {
	// Look for {tmdb-12345} pattern in path
	re := regexp.MustCompile(`\{tmdb-(\d+)\}`)
	matches := re.FindStringSubmatch(filePath)
	if len(matches) > 1 {
		if id, err := strconv.Atoi(matches[1]); err == nil {
			return id
		}
	}
	return 0
}

// getItemMetadata fetches detailed metadata for a specific item by ratingKey
func (c *Client) getItemMetadata(ratingKey string) (*MediaContainer, error) {
	req, err := http.NewRequest("GET", c.baseURL+"/library/metadata/"+ratingKey, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", c.token)
	req.Header.Set("Accept", "application/xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch item metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex returned status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var container MediaContainer
	if err := xml.Unmarshal(body, &container); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &container, nil
}
