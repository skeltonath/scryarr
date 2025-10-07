package tmdb

import (
	"fmt"

	tmdb "github.com/cyruzin/golang-tmdb"
	"github.com/dppeppel/scryarr/internal/logging"
	"github.com/dppeppel/scryarr/internal/store"
	"github.com/rs/zerolog"
)

var log zerolog.Logger

func init() {
	log = logging.GetLogger("tmdb")
}

// Client wraps the TMDb API client with caching
type Client struct {
	client *tmdb.Client
	store  *store.Store
}

// NewClient creates a new TMDb client
func NewClient(apiKey string, store *store.Store) (*Client, error) {
	tmdbClient, err := tmdb.Init(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize TMDb client: %w", err)
	}

	return &Client{
		client: tmdbClient,
		store:  store,
	}, nil
}

// TitleResult represents a resolved title with metadata
type TitleResult struct {
	TMDbID     int
	IMDbID     string
	Title      string
	Year       int
	MediaType  string // movie or tv
	Overview   string
	Genres     []string
	Keywords   []string
	VoteCount  int
	VoteAvg    float64
	RuntimeMin int
	Country    string
}

// SearchAndResolve searches for a title and returns the best match
func (c *Client) SearchAndResolve(title string, year int, mediaType string) (*TitleResult, error) {
	// Check cache first
	if cached := c.getCached(title, year, mediaType); cached != nil {
		log.Debug().Str("title", title).Int("year", year).Msg("cache hit")
		return cached, nil
	}

	log.Info().Str("title", title).Int("year", year).Str("type", mediaType).Msg("searching TMDb")

	var result *TitleResult
	var err error

	if mediaType == "movie" {
		result, err = c.searchMovie(title, year)
	} else if mediaType == "tv" {
		result, err = c.searchTV(title, year)
	} else {
		return nil, fmt.Errorf("unknown media type: %s", mediaType)
	}

	if err != nil {
		return nil, err
	}

	// Cache the result
	c.cacheResult(result)

	return result, nil
}

func (c *Client) searchMovie(title string, year int) (*TitleResult, error) {
	opts := map[string]string{"year": fmt.Sprintf("%d", year)}
	results, err := c.client.GetSearchMovies(title, opts)
	if err != nil {
		return nil, fmt.Errorf("TMDb search failed: %w", err)
	}

	if len(results.Results) == 0 {
		return nil, fmt.Errorf("no results found for %s (%d)", title, year)
	}

	// Take the first result (best match)
	movie := results.Results[0]

	// Get detailed info
	details, err := c.client.GetMovieDetails(int(movie.ID), nil)
	if err != nil {
		log.Warn().Err(err).Int("id", int(movie.ID)).Msg("failed to get movie details")
		// Continue with basic info
	}

	// Get keywords
	keywords, err := c.client.GetMovieKeywords(int(movie.ID))
	var keywordList []string
	if err == nil && keywords != nil {
		for _, kw := range keywords.Keywords {
			keywordList = append(keywordList, kw.Name)
		}
	}

	result := &TitleResult{
		TMDbID:    int(movie.ID),
		Title:     movie.Title,
		Year:      year,
		MediaType: "movie",
		Overview:  movie.Overview,
		VoteCount: int(movie.VoteCount),
		VoteAvg:   float64(movie.VoteAverage),
		Keywords:  keywordList,
	}

	if details != nil {
		result.IMDbID = details.IMDbID
		result.RuntimeMin = int(details.Runtime)

		// Extract genres
		for _, g := range details.Genres {
			result.Genres = append(result.Genres, g.Name)
		}

		// Extract country
		if len(details.ProductionCountries) > 0 {
			result.Country = details.ProductionCountries[0].Iso3166_1
		}
	}

	return result, nil
}

func (c *Client) searchTV(title string, year int) (*TitleResult, error) {
	opts := map[string]string{"first_air_date_year": fmt.Sprintf("%d", year)}
	results, err := c.client.GetSearchTVShow(title, opts)
	if err != nil {
		return nil, fmt.Errorf("TMDb search failed: %w", err)
	}

	if len(results.Results) == 0 {
		return nil, fmt.Errorf("no results found for %s (%d)", title, year)
	}

	// Take the first result
	show := results.Results[0]

	// Get detailed info
	details, err := c.client.GetTVDetails(int(show.ID), nil)
	if err != nil {
		log.Warn().Err(err).Int("id", int(show.ID)).Msg("failed to get TV details")
	}

	// Get keywords
	keywords, err := c.client.GetTVKeywords(int(show.ID))
	var keywordList []string
	if err == nil && keywords != nil {
		for _, kw := range keywords.Results {
			keywordList = append(keywordList, kw.Name)
		}
	}

	result := &TitleResult{
		TMDbID:    int(show.ID),
		Title:     show.Name,
		Year:      year,
		MediaType: "tv",
		Overview:  show.Overview,
		VoteCount: int(show.VoteCount),
		VoteAvg:   float64(show.VoteAverage),
		Keywords:  keywordList,
	}

	if details != nil {
		// Extract genres
		for _, g := range details.Genres {
			result.Genres = append(result.Genres, g.Name)
		}

		// Extract runtime (average episode runtime)
		if len(details.EpisodeRunTime) > 0 {
			result.RuntimeMin = int(details.EpisodeRunTime[0])
		}

		// Extract country
		if len(details.OriginCountry) > 0 {
			result.Country = details.OriginCountry[0]
		}
	}

	return result, nil
}

func (c *Client) getCached(title string, year int, mediaType string) *TitleResult {
	if c.store == nil {
		return nil
	}

	cached, err := c.store.GetTitleResolution(title, year, mediaType)
	if err != nil || cached == nil {
		return nil
	}

	return &TitleResult{
		TMDbID:     cached.TMDbID,
		IMDbID:     cached.IMDbID,
		Title:      cached.Title,
		Year:       cached.Year,
		MediaType:  cached.MediaType,
		RuntimeMin: cached.RuntimeMin,
		Country:    cached.Country,
	}
}

func (c *Client) cacheResult(result *TitleResult) {
	if c.store == nil || result == nil {
		return
	}

	tr := &store.TitleResolution{
		Title:      result.Title,
		Year:       result.Year,
		MediaType:  result.MediaType,
		TMDbID:     result.TMDbID,
		IMDbID:     result.IMDbID,
		Country:    result.Country,
		RuntimeMin: result.RuntimeMin,
	}

	if err := c.store.CacheTitleResolution(tr); err != nil {
		log.Warn().Err(err).Msg("failed to cache title resolution")
	}
}
