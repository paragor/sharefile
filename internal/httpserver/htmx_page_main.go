package httpserver

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/paragor/sharefile/internal/storage"
)

type listContext struct {
	Files []listContextFile
}
type listContextFile struct {
	Id             string
	Path           string
	LastModifiedAt time.Time
	SizeHuman      string
}

type mainContext struct {
	AuthCompleted    bool
	Email            string
	ImpersonateEmail string
	RssLink          string
	ShareLink        string

	ChildComponent any
}

func (s *httpServer) htmxPrepareMainContext(r *http.Request) *mainContext {
	auth, err := s.extractAuthContext(r)
	if err != nil {
		return &mainContext{
			AuthCompleted:    false,
			Email:            "",
			ImpersonateEmail: "",
		}
	}
	return &mainContext{
		AuthCompleted:    true,
		Email:            auth.Email,
		ImpersonateEmail: auth.ImpersonateEmail,
		ChildComponent:   nil,
	}
}
func (s *httpServer) htmxPageMain(w http.ResponseWriter, r *http.Request) {
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

	meta, err := userStorage.GetMetadata(r.Context())
	if err != nil {
		httpError(r.Context(), w, "unable to fetch metadata", err, http.StatusInternalServerError)
		return
	}

	listFilesHtml, err := s.htmxComponentListFiles(r.Context(), userStorage)
	if err != nil {
		httpError(r.Context(), w, "error on render list component", err, http.StatusInternalServerError)
		return
	}

	uploadForm, err := renderHtmx("component/upload_form", nil)
	if err != nil {
		httpError(r.Context(), w, "error on render upload form", err, http.StatusInternalServerError)
		return
	}

	renderContext := s.htmxPrepareMainContext(r)
	renderContext.ChildComponent = template.HTML(uploadForm.String()) + listFilesHtml

	renderContext.RssLink = s.getRssLink(meta)
	renderContext.ShareLink = s.getShareLink(meta)

	writeHtmx(w, r, "page/index", renderContext, 200)
}

func (s *httpServer) htmxPageLogin(w http.ResponseWriter, r *http.Request) {
	oidcHtmx, err := renderHtmx("component/auth_oidc_challenge", "")
	if err != nil {
		httpError(r.Context(), w, "error on render oidc auth", err, http.StatusInternalServerError)
		return
	}
	renderContext := s.htmxPrepareMainContext(r)
	renderContext.ChildComponent = template.HTML(oidcHtmx.String())

	writeHtmx(w, r, "page/index", renderContext, http.StatusUnauthorized)
}

func (s *httpServer) htmxComponentListFiles(ctx context.Context, userScopedStorage storage.UserScopedStorage) (template.HTML, error) {
	listing, err := userScopedStorage.ListFiles(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to get files listing: %w", err)
	}

	renderListing := make([]listContextFile, 0, len(listing))
	for _, file := range listing {
		renderListing = append(renderListing, listContextFile{
			Id:             uuid.New().String(),
			Path:           file.Path,
			LastModifiedAt: file.LastModifiedAt,
			SizeHuman:      bytesConvert(file.Size),
		})
	}

	result, err := renderHtmx("component/list_files", listContext{Files: renderListing})
	if err != nil {
		return "", fmt.Errorf("fail to render: %w", err)
	}
	return template.HTML(result.String()), nil
}

func (s *httpServer) getRssLink(meta *storage.Metadata) string {
	return fmt.Sprintf("%s/rss/%s/%s", s.serverPublicUrl, meta.Email, meta.Secret)
}
func (s *httpServer) getShareLink(meta *storage.Metadata) string {
	return fmt.Sprintf("%s/share/%s/%s", s.serverPublicUrl, meta.Email, meta.Secret)
}

func bytesConvert(bytes int) string {
	if bytes == 0 {
		return "0 bytes"
	}

	base := math.Floor(math.Log(float64(bytes)) / math.Log(1024))
	units := []string{"bytes", "KiB", "MiB", "GiB"}

	stringVal := fmt.Sprintf("%.2f", float64(bytes)/math.Pow(1024, base))
	stringVal = strings.TrimSuffix(stringVal, ".00")
	return fmt.Sprintf("%s %v",
		stringVal,
		units[int(base)],
	)
}
