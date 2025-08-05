package httpserver

import (
	"encoding/json"
	"html/template"
	"net/http"
	"time"
)

type whoAmIContext struct {
	Expiration      time.Duration
	TokenPrettyJson string
	Email           string
}

func (s *httpServer) htmxPageWhoami(w http.ResponseWriter, r *http.Request) {
	auth, err := s.extractAuthContext(r)
	if err != nil {
		httpError(r.Context(), w, "error on getting auth context", err, http.StatusInternalServerError)
		return
	}

	token, err := json.MarshalIndent(auth.RawToken, "", "  ")
	if err != nil {
		httpError(r.Context(), w, "error on marshal token", err, http.StatusInternalServerError)
		return
	}

	whoamiHtmx, err := renderHtmx("component/whoami", whoAmIContext{
		Expiration:      auth.ExpireAt.Sub(time.Now()),
		TokenPrettyJson: string(token),
		Email:           auth.Email,
	})
	if err != nil {
		httpError(r.Context(), w, "error on render whoami component", err, http.StatusInternalServerError)
		return
	}

	renderContext := s.htmxPrepareMainContext(r)
	renderContext.ChildComponent = template.HTML(whoamiHtmx.String())

	writeHtmx(w, r, "page/index", renderContext, http.StatusOK)
}
