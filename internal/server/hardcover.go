package server

import (
	"context"

	"github.com/user/bunbu-shelf/internal/hardcover"
)

func searchHardcover(ctx context.Context, apiKey, q string) []remoteSearchResult {
	client := hardcover.New(apiKey)
	raw, err := client.Search(ctx, q)
	if err != nil {
		return nil
	}
	var out []remoteSearchResult
	for _, r := range raw {
		author := ""
		if len(r.Authors) > 0 {
			author = r.Authors[0]
		}
		out = append(out, remoteSearchResult{
			Title:    r.Title,
			Author:   author,
			Year:     r.Year,
			ISBN:     r.ISBN,
			CoverURL: r.CoverURL,
			Pages:    r.Pages,
		})
	}
	return out
}
