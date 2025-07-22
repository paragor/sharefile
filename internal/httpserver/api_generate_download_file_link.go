package httpserver

import (
	"fmt"
	"net/http"
	"time"
)

func (s *httpServer) apiGenerateDownloadFileLink(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		httpError(r.Context(), w, "query param 'path' is empty", fmt.Errorf("no path in query"), http.StatusBadRequest)
	}
	email, err := s.extractEmail(r)
	if err != nil {
		httpError(r.Context(), w, "cant read email from request", err, http.StatusInternalServerError)
		return
	}

	userStorage, err := s.storage.OpenStorage(r.Context(), email, true)
	if err != nil {
		httpError(r.Context(), w, "unable to open user scoped storage", err, http.StatusInternalServerError)
		return
	}

	link, err := userStorage.GenerateDownloadLink(r.Context(), filePath, 15*time.Minute)
	if err != nil {
		httpError(r.Context(), w, "unable to generate download link", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", link)
	w.Header().Set("Content-Type", "text/uri-list")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(link))
}
