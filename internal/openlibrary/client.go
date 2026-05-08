package openlibrary

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	searchBaseURL = "https://openlibrary.org/search.json"
	coverBaseURL  = "https://covers.openlibrary.org/b"
)

// SearchResult holds metadata from an Open Library search hit.
type SearchResult struct {
	Key       string
	Title     string
	Authors   []string
	ISBN      string // first 13-digit ISBN, if available
	CoverID   int
	Year      int
}

// Client calls the Open Library API.
type Client struct {
	http     *http.Client
	coverDir string
}

// New creates a Client that saves covers to coverDir.
func New(coverDir string) *Client {
	return &Client{
		http:     &http.Client{Timeout: 10 * time.Second},
		coverDir: coverDir,
	}
}

// SearchByTitle searches Open Library by title (and optional author).
func (c *Client) SearchByTitle(ctx context.Context, q string) ([]SearchResult, error) {
	params := url.Values{
		"q":      {q},
		"fields": {"key,title,author_name,isbn,cover_i,first_publish_year"},
		"limit":  {"10"},
	}
	return c.search(ctx, params)
}

// SearchByISBN fetches a single book by ISBN from Open Library.
func (c *Client) SearchByISBN(ctx context.Context, isbn string) (*SearchResult, error) {
	params := url.Values{
		"isbn":   {isbn},
		"fields": {"key,title,author_name,isbn,cover_i,first_publish_year"},
		"limit":  {"1"},
	}
	results, err := c.search(ctx, params)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

func (c *Client) search(ctx context.Context, params url.Values) ([]SearchResult, error) {
	reqURL := searchBaseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "bunbu-shelf/1.0 (https://github.com/user/bunbu-shelf)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open library search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open library search: status %d", resp.StatusCode)
	}

	var body struct {
		Docs []struct {
			Key              string   `json:"key"`
			Title            string   `json:"title"`
			AuthorName       []string `json:"author_name"`
			ISBN             []string `json:"isbn"`
			CoverI           int      `json:"cover_i"`
			FirstPublishYear int      `json:"first_publish_year"`
		} `json:"docs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	var out []SearchResult
	for _, d := range body.Docs {
		r := SearchResult{
			Key:     d.Key,
			Title:   d.Title,
			Authors: d.AuthorName,
			CoverID: d.CoverI,
			Year:    d.FirstPublishYear,
		}
		for _, isbn := range d.ISBN {
			if len(isbn) == 13 {
				r.ISBN = isbn
				break
			}
		}
		if r.ISBN == "" && len(d.ISBN) > 0 {
			r.ISBN = d.ISBN[0]
		}
		out = append(out, r)
	}
	return out, nil
}

// FetchCover downloads a cover image by Open Library cover ID and saves it
// to coverDir/<slug>.jpg. Returns the relative path (covers/<slug>.jpg).
func (c *Client) FetchCover(ctx context.Context, coverID int, slug string) (string, error) {
	if coverID <= 0 {
		return "", nil
	}
	imgURL := fmt.Sprintf("%s/id/%d-L.jpg", coverBaseURL, coverID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch cover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch cover: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// A 1x1 pixel response from OL means "no cover".
	if len(data) < 1000 {
		return "", nil
	}

	filename := slug + ".jpg"
	path := fmt.Sprintf("%s/%s", c.coverDir, filename)
	if err := writeFile(path, data); err != nil {
		return "", err
	}
	return "covers/" + filename, nil
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}
