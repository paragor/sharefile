package httpserver

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

type sharePageContext struct {
	Files []sharePageFile
}
type sharePageFile struct {
	Path      string
	Link      string
	SizeHuman string
}

func (s *httpServer) htmxPageShare(w http.ResponseWriter, r *http.Request) {
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
	if rssSecret != meta.Secret {
		httpError(r.Context(), w, "invalid rss secret", err, http.StatusUnauthorized)
		return
	}

	listing, err := userStorage.ListFiles(r.Context())
	if err != nil {
		httpError(r.Context(), w, "unable to list files", err, http.StatusInternalServerError)
		return
	}

	sharePage := &sharePageContext{}
	for _, fileMeta := range listing {
		link, err := userStorage.GenerateDownloadLink(r.Context(), fileMeta.Path, s.rssExpirationLink)
		if err != nil {
			httpError(r.Context(), w, "unable to generate links for files", err, http.StatusInternalServerError)
			return
		}
		sharePage.Files = append(sharePage.Files, sharePageFile{
			Path:      fileMeta.Path,
			Link:      link,
			SizeHuman: bytesConvert(fileMeta.Size),
		})
	}

	sharePageHtml, err := renderHtmx("component/share", sharePage)
	if err != nil {
		httpError(r.Context(), w, "error on render upload form", err, http.StatusInternalServerError)
		return
	}

	renderContext := s.htmxPrepareMainContext(r)
	renderContext.ChildComponent = template.HTML(sharePageHtml.String())
	writeHtmx(w, r, "page/index", renderContext, 200)
}
