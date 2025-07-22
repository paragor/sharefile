package httpserver

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (s *httpServer) apiUploadFile(w http.ResponseWriter, r *http.Request) {
	email, err := s.extractEmail(r)
	if err != nil {
		httpError(r.Context(), w, "cant read email from request", err, http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()
	if err := r.ParseMultipartForm(1024 * 1024 * 10); err != nil {
		httpError(r.Context(), w, "Cant parse multipart form", err, http.StatusBadRequest)
		return
	}

	fileContentType := "application/octet-stream"
	var body io.ReadCloser
	var filePath string
	for _, fh := range r.MultipartForm.File["file"] {
		ct := fh.Header.Get("Content-Type")
		if ct != "" {
			fileContentType = ct
		}
		filePath = fh.Filename
		f, err := fh.Open()
		if err != nil {
			httpError(r.Context(), w, "Cant open file from multipart form", err, http.StatusInternalServerError)
			return
		}
		body = f
		break
	}
	if body == nil {
		httpError(r.Context(), w, "Cant find file in multipart form", err, http.StatusBadRequest)
		return
	}
	defer body.Close()

	if strings.Contains(filePath, "/") {
		httpError(r.Context(), w, "file path should not contain '/' character", fmt.Errorf(
			"file path '%s' contain bad characters",
			filePath,
		), http.StatusBadRequest)
		return
	}

	userStorage, err := s.storage.OpenStorage(r.Context(), email, true)
	if err != nil {
		httpError(r.Context(), w, "unable to open user scoped storage", err, http.StatusInternalServerError)
		return
	}
	if err := userStorage.Upload(r.Context(), filePath, fileContentType, body); err != nil {
		httpError(r.Context(), w, "error on upload file", err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
