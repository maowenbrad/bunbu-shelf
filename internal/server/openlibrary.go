package server

import (
	"context"
	"fmt"

	"github.com/user/bunbu-shelf/internal/openlibrary"
)

func searchOpenLibrary(ctx context.Context, q string) []remoteSearchResult {
	client := openlibrary.New("")
	raw, err := client.SearchByTitle(ctx, q)
	if err != nil {
		return nil
	}
	var out []remoteSearchResult
	for _, r := range raw {
		author := ""
		if len(r.Authors) > 0 {
			author = r.Authors[0]
		}
		coverURL := ""
		if r.CoverID > 0 {
			coverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-S.jpg", r.CoverID)
		}
		out = append(out, remoteSearchResult{
			Title:     r.Title,
			Author:    author,
			Year:      r.Year,
			ISBN:      r.ISBN,
			CoverID:   r.CoverID,
			CoverURL:  coverURL,
			Publisher: r.Publisher,
			Pages:     r.Pages,
		})
	}
	return out
}
