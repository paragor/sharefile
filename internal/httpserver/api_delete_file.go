package httpserver

import (
	"fmt"
	"net/http"
)

func (s *httpServer) apiDelteFile(w http.ResponseWriter, r *http.Request) {
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

	err = userStorage.Delete(r.Context(), filePath)
	if err != nil {
		httpError(r.Context(), w, "unable to delete file", err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(""))
}
