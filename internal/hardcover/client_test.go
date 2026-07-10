package hardcover

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchParsesBothResultShapes(t *testing.T) {
	typesenseEnvelope := `{
		"found": 2,
		"hits": [
			{"document": {
				"title": "Dune",
				"author_names": ["Frank Herbert"],
				"isbns": ["0441013597", "9780441013593"],
				"release_year": 1965,
				"pages": 412,
				"image": {"url": "https://assets.hardcover.app/dune.jpg"}
			}},
			{"document": {
				"title": "Dune Messiah",
				"author_names": ["Frank Herbert"],
				"isbns": [],
				"release_year": 1969,
				"image": null
			}}
		]
	}`
	bareArray := `[
		{
			"title": "Dune",
			"author_names": ["Frank Herbert"],
			"isbns": ["0441013597", "9780441013593"],
			"release_year": 1965,
			"pages": 412,
			"image": "https://assets.hardcover.app/dune.jpg"
		},
		{
			"title": "Dune Messiah",
			"author_names": ["Frank Herbert"],
			"isbns": [],
			"release_year": 1969
		}
	]`

	cases := []struct {
		name    string
		results string
	}{
		{"typesense envelope, image object", typesenseEnvelope},
		{"bare array, image string", bareArray},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
					t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
				}
				var req struct {
					Variables map[string]string `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if req.Variables["query"] != "dune" {
					t.Errorf("query variable = %q, want %q", req.Variables["query"], "dune")
				}
				fmt.Fprintf(w, `{"data": {"search": {"results": %s}}}`, tc.results)
			}))
			defer srv.Close()

			c := New("test-key")
			c.apiURL = srv.URL

			got, err := c.Search(context.Background(), "dune")
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("got %d results, want 2", len(got))
			}
			first := got[0]
			if first.Title != "Dune" || first.Year != 1965 || first.Pages != 412 {
				t.Errorf("first result = %+v", first)
			}
			if first.ISBN != "9780441013593" {
				t.Errorf("ISBN = %q, want 13-digit preferred", first.ISBN)
			}
			if first.CoverURL != "https://assets.hardcover.app/dune.jpg" {
				t.Errorf("CoverURL = %q", first.CoverURL)
			}
			second := got[1]
			if second.ISBN != "" || second.CoverURL != "" {
				t.Errorf("second result should have empty ISBN and CoverURL, got %+v", second)
			}
		})
	}
}

func TestSearchGraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errors": [{"message": "invalid token"}]}`)
	}))
	defer srv.Close()

	c := New("bad-key")
	c.apiURL = srv.URL

	if _, err := c.Search(context.Background(), "dune"); err == nil {
		t.Fatal("want error for GraphQL errors response, got nil")
	}
}

func TestNewStripsBearerPrefix(t *testing.T) {
	c := New("Bearer  abc123 ")
	if c.apiKey != "abc123" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "abc123")
	}
	c = New("abc123")
	if c.apiKey != "abc123" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "abc123")
	}
}
