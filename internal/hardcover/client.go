// Package hardcover is a thin client for the hardcover.app GraphQL API's
// book search. Unlike Open Library it requires an API key (free, from
// https://hardcover.app/account/api), passed as a Bearer token.
package hardcover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultAPIURL = "https://api.hardcover.app/v1/graphql"

// searchQuery drives Hardcover's Typesense-backed search — the same index
// the website uses, so result ranking matches what a user sees there. The
// results field is a raw JSON blob, decoded leniently in parseResults.
const searchQuery = `query Search($query: String!) {
  search(query: $query, query_type: "Book", per_page: 10, page: 1) {
    results
  }
}`

// SearchResult holds metadata from a Hardcover search hit.
type SearchResult struct {
	Title    string
	Authors  []string
	ISBN     string // first 13-digit ISBN, if available
	Year     int
	Pages    int
	CoverURL string
}

// Client calls the Hardcover API.
type Client struct {
	http   *http.Client
	apiKey string
	apiURL string
}

// New creates a Client. The key is accepted with or without the "Bearer "
// prefix that Hardcover's account page includes when displaying it.
func New(apiKey string) *Client {
	return &Client{
		http:   &http.Client{Timeout: 10 * time.Second},
		apiKey: strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(apiKey), "Bearer ")),
		apiURL: defaultAPIURL,
	}
}

// Search queries Hardcover's book search by title/author/ISBN keywords.
func (c *Client) Search(ctx context.Context, q string) ([]SearchResult, error) {
	payload, err := json.Marshal(map[string]any{
		"query":     searchQuery,
		"variables": map[string]string{"query": q},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", "bunbu-shelf/1.0 (https://github.com/user/bunbu-shelf)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hardcover search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hardcover search: status %d", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Search struct {
				Results json.RawMessage `json:"results"`
			} `json:"search"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	if len(body.Errors) > 0 {
		return nil, fmt.Errorf("hardcover search: %s", body.Errors[0].Message)
	}

	return parseResults(body.Data.Search.Results)
}

// document is one book from the search index. Only the fields bunbu-shelf
// uses are decoded; unknown fields are ignored.
type document struct {
	Title       string   `json:"title"`
	AuthorNames []string `json:"author_names"`
	ISBNs       []string `json:"isbns"`
	ReleaseYear int      `json:"release_year"`
	Pages       int      `json:"pages"`
	Image       coverURL `json:"image"`
}

// coverURL absorbs both shapes the search index has used for the cover:
// a plain URL string, or an object with a "url" field. null decodes to "".
type coverURL string

func (c *coverURL) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*c = coverURL(s)
		return nil
	}
	var obj struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		*c = coverURL(obj.URL)
		return nil
	}
	*c = ""
	return nil
}

// parseResults decodes the raw search results blob. Hardcover's API is
// still evolving, so both known layouts are accepted: the Typesense
// envelope ({"hits": [{"document": {...}}, ...]}) and a bare array of
// documents. Individual hits that fail to decode are skipped rather than
// failing the whole search.
func parseResults(raw json.RawMessage) ([]SearchResult, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var docs []json.RawMessage

	var envelope struct {
		Hits []struct {
			Document json.RawMessage `json:"document"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.Hits) > 0 {
		for _, h := range envelope.Hits {
			docs = append(docs, h.Document)
		}
	} else {
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, fmt.Errorf("unrecognized search results shape")
		}
		docs = arr
	}

	var out []SearchResult
	for _, d := range docs {
		var doc document
		if err := json.Unmarshal(d, &doc); err != nil || doc.Title == "" {
			continue
		}
		r := SearchResult{
			Title:    doc.Title,
			Authors:  doc.AuthorNames,
			Year:     doc.ReleaseYear,
			Pages:    doc.Pages,
			CoverURL: string(doc.Image),
		}
		for _, isbn := range doc.ISBNs {
			if len(isbn) == 13 {
				r.ISBN = isbn
				break
			}
		}
		if r.ISBN == "" && len(doc.ISBNs) > 0 {
			r.ISBN = doc.ISBNs[0]
		}
		out = append(out, r)
	}
	return out, nil
}
