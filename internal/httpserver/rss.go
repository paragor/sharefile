package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/feeds"
)

func (s *httpServer) generateRSS(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 2 {
		httpError(
			r.Context(),
			w,
			"invalid url",
			fmt.Errorf("invalid url for rss: %s", r.URL.Path),
			http.StatusInternalServerError,
		)
		return
	}

	email, rssSecret := parts[len(parts)-2], parts[len(parts)-1]
	userStorage, err := s.storage.OpenStorage(r.Context(), email, false)
	if err != nil {
		httpError(r.Context(), w, "unable communicate with storage", err, http.StatusInternalServerError)
		return
	}

	meta, err := userStorage.GetMetadata(r.Context())
	if err != nil {
		httpError(r.Context(), w, "cant read metadata from storage", err, http.StatusInternalServerError)
		return
	}
	if rssSecret != meta.RssSecret {
		httpError(r.Context(), w, "invalid rss secret", err, http.StatusUnauthorized)
		return
	}

	listing, err := userStorage.ListFiles(r.Context())
	if err != nil {
		httpError(r.Context(), w, "unable to list files", err, http.StatusInternalServerError)
		return
	}

	feed := &feeds.Feed{
		Title:       "Share File Of " + email,
		Description: "Shared files",
		Author:      &feeds.Author{Name: email, Email: email},
		Created:     time.Now(),
	}

	for _, fileMeta := range listing {
		link, err := userStorage.GenerateDownloadLink(r.Context(), fileMeta.Path, s.rssExpirationLink)
		if err != nil {
			httpError(r.Context(), w, "unable to generate links for files", err, http.StatusInternalServerError)
			return
		}
		feed.Add(&feeds.Item{
			Title: fileMeta.Path,
			Link: &feeds.Link{
				Href: link,
			},
			Updated: fileMeta.LastModifiedAt,
			Created: fileMeta.LastModifiedAt,
		})
	}

	rss, err := feed.ToRss()
	if err != nil {
		httpError(r.Context(), w, "unable to build rss feed", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(rss))
}
