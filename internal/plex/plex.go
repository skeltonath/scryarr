package plex

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
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
	Title     string `xml:"title,attr"`
	Year      int    `xml:"year,attr"`
	Type      string `xml:"type,attr"`
	RatingKey string `xml:"ratingKey,attr"`
	GUID      []GUID `xml:"Guid"`
}

type Directory struct {
	Title     string `xml:"title,attr"`
	Year      int    `xml:"year,attr"`
	Type      string `xml:"type,attr"`
	RatingKey string `xml:"ratingKey,attr"`
	GUID      []GUID `xml:"Guid"`
}

type GUID struct {
	ID string `xml:"id,attr"`
}

// GetInventory fetches all movies and TV shows from Plex library
func (c *Client) GetInventory() ([]MediaItem, error) {
	log.Info().Msg("fetching Plex library inventory")

	var allItems []MediaItem

	// Fetch movies
	movies, err := c.getLibrarySection("movie")
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch movies, continuing")
	} else {
		allItems = append(allItems, movies...)
	}

	// Fetch TV shows
	shows, err := c.getLibrarySection("show")
	if err != nil {
		log.Warn().Err(err).Msg("failed to fetch TV shows, continuing")
	} else {
		allItems = append(allItems, shows...)
	}

	log.Info().Int("count", len(allItems)).Msg("fetched Plex inventory")
	return allItems, nil
}

func (c *Client) getLibrarySection(mediaType string) ([]MediaItem, error) {
	// Get all library sections
	sections, err := c.getLibrarySections()
	if err != nil {
		return nil, err
	}

	var items []MediaItem
	for _, section := range sections {
		if section.Type == mediaType {
			sectionItems, err := c.getLibrarySectionContents(section.Key)
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

func (c *Client) getLibrarySectionContents(sectionKey string) ([]MediaItem, error) {
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
		// Parse GUIDs
		for _, guid := range v.GUID {
			if tmdbID := parseTMDbID(guid.ID); tmdbID > 0 {
				item.TMDbID = tmdbID
			}
			if imdbID := parseIMDbID(guid.ID); imdbID != "" {
				item.IMDbID = imdbID
			}
		}
		if item.TMDbID > 0 {
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
		// Parse GUIDs
		for _, guid := range d.GUID {
			if tmdbID := parseTMDbID(guid.ID); tmdbID > 0 {
				item.TMDbID = tmdbID
			}
			if imdbID := parseIMDbID(guid.ID); imdbID != "" {
				item.IMDbID = imdbID
			}
		}
		if item.TMDbID > 0 {
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
