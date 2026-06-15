// Package allenbrain is the library behind the allenbrain command line:
// the HTTP client, request shaping, and typed data models for the Allen
// Brain Atlas public API (api.brain-map.org/api/v2).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries
// the transient failures (429 and 5xx) that any public API throws under load.
package allenbrain

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultUserAgent identifies the client to Allen Brain Atlas.
const DefaultUserAgent = "allenbrain-cli/dev (+https://github.com/tamnd/allenbrain-cli)"

// Host is the site this client talks to.
const Host = "api.brain-map.org"

// BaseURL is the root every request is built from.
const BaseURL = "http://api.brain-map.org/api/v2"

// BrainURL is the human-facing site URL for gene pages.
const BrainURL = "https://mouse.brain-map.org"

// Client talks to the Allen Brain Atlas API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 15s timeout, a 300ms
// minimum gap between requests, and three retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		UserAgent: DefaultUserAgent,
		BaseURL:   BaseURL,
		Rate:      300 * time.Millisecond,
		Retries:   3,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings. The caller owns nothing extra; the body is read
// fully and closed here.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire types (unexported) ---

type wireResponse[T any] struct {
	Success   bool `json:"success"`
	TotalRows int  `json:"total_rows"`
	Msg       []T  `json:"msg"`
}

type wireGene struct {
	ID       int    `json:"id"`
	Acronym  string `json:"acronym"`
	Name     string `json:"name"`
	EntrezID int    `json:"entrez_id"`
}

type wireAtlas struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ImageType string `json:"image_type"`
}

type wireDataset struct {
	ID           int         `json:"id"`
	PlaneSection int         `json:"plane_of_section_id"`
	RefSpace     int         `json:"reference_space_id"`
	Genes        []wireGene  `json:"genes"`
}

// --- public types ---

// Gene is one gene entry from the Allen Brain Atlas.
type Gene struct {
	ID      int    `json:"id" kit:"id"`
	Acronym string `json:"acronym"`
	Name    string `json:"name"`
	EntrezID int   `json:"entrez_id,omitempty"`
}

// Atlas is one reference atlas from the Allen Brain Atlas.
type Atlas struct {
	ID        int    `json:"id" kit:"id"`
	Name      string `json:"name"`
	ImageType string `json:"image_type,omitempty"`
}

// Dataset is one section dataset from the Allen Brain Atlas.
type Dataset struct {
	ID           int    `json:"id" kit:"id"`
	PlaneSection int    `json:"plane_section_id,omitempty"`
	Genes        []Gene `json:"genes,omitempty"`
}

// --- API methods ---

// Genes lists genes with pagination.
func (c *Client) Genes(ctx context.Context, limit, start int) ([]Gene, error) {
	u := c.BaseURL + "/data/Gene/query.json?" +
		"num_rows=" + strconv.Itoa(limit) +
		"&start_row=" + strconv.Itoa(start)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp wireResponse[wireGene]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode genes: %w", err)
	}
	return toGenes(resp.Msg), nil
}

// SearchGenes searches genes by name using the criteria filter.
func (c *Client) SearchGenes(ctx context.Context, query string, limit int) ([]Gene, error) {
	criteria := "[name$il'*" + query + "*']"
	u := c.BaseURL + "/data/Gene/query.json?" +
		"criteria=" + url.QueryEscape(criteria) +
		"&num_rows=" + strconv.Itoa(limit)
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp wireResponse[wireGene]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode search genes: %w", err)
	}
	return toGenes(resp.Msg), nil
}

// Atlases lists all reference atlases.
func (c *Client) Atlases(ctx context.Context) ([]Atlas, error) {
	u := c.BaseURL + "/data/Atlas/query.json?num_rows=100"
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp wireResponse[wireAtlas]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode atlases: %w", err)
	}
	return toAtlases(resp.Msg), nil
}

// Datasets lists section datasets with pagination.
func (c *Client) Datasets(ctx context.Context, limit, start int) ([]Dataset, error) {
	u := c.BaseURL + "/data/SectionDataSet/query.json?" +
		"criteria=products[abbreviation$eq'Mouse']" +
		"&num_rows=" + strconv.Itoa(limit) +
		"&start_row=" + strconv.Itoa(start) +
		"&include=genes"
	body, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var resp wireResponse[wireDataset]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode datasets: %w", err)
	}
	return toDatasets(resp.Msg), nil
}

// --- converters ---

func toGenes(ws []wireGene) []Gene {
	out := make([]Gene, len(ws))
	for i, w := range ws {
		out[i] = Gene{
			ID:       w.ID,
			Acronym:  w.Acronym,
			Name:     w.Name,
			EntrezID: w.EntrezID,
		}
	}
	return out
}

func toAtlases(ws []wireAtlas) []Atlas {
	out := make([]Atlas, len(ws))
	for i, w := range ws {
		out[i] = Atlas{
			ID:        w.ID,
			Name:      w.Name,
			ImageType: w.ImageType,
		}
	}
	return out
}

func toDatasets(ws []wireDataset) []Dataset {
	out := make([]Dataset, len(ws))
	for i, w := range ws {
		d := Dataset{
			ID:           w.ID,
			PlaneSection: w.PlaneSection,
		}
		if len(w.Genes) > 0 {
			d.Genes = toGenes(w.Genes)
		}
		out[i] = d
	}
	return out
}
