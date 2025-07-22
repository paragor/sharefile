package httpserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/paragor/sharefile/internal/httpserver/public"
	"github.com/paragor/sharefile/internal/log"
	"github.com/paragor/sharefile/internal/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server interface {
	ListenAndServe() error
	Shutdown(ctx context.Context) error
}

type httpServer struct {
	rssExpirationLink time.Duration
	storage           storage.Storage
	oidc              *authOidcContext
	serverPublicUrl   string

	mux    *mux.Router
	server *http.Server
}

func httpError(ctx context.Context, w http.ResponseWriter, publicMsg string, err error, code int) {
	log.FromContext(ctx).With(log.Error(err), slog.Int("response_code", code)).Error(publicMsg)
	http.Error(w, publicMsg, code)
}

func restartEtag(handler http.Handler) http.Handler {
	start := time.Now()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		etag := start.String() + "@" + path.Clean(request.URL.Path)
		if requestEtag := request.Header.Get("If-None-Match"); requestEtag == etag {
			writer.WriteHeader(304)
			return
		}
		writer.Header().Set("ETag", etag)
		handler.ServeHTTP(writer, request)
	})
}

func cacheMiddleware(handler http.Handler, duration time.Duration) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "max-age="+strconv.Itoa(int(duration.Seconds())))
		handler.ServeHTTP(writer, request)
	})
}

func (s *httpServer) apiPing(writer http.ResponseWriter, request *http.Request) {
	writer.WriteHeader(200)
}
func (s *httpServer) ListenAndServe() error {
	return s.server.ListenAndServe()
}

func (s *httpServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func NewHttpServer(
	listen string,
	storage storage.Storage,
	authConfig *AuthOidcConfig,
	serverPublicUrl string,
	diagnosticEndpointsEnabled bool,
	rssExpirationLink time.Duration,
) (Server, error) {
	if rssExpirationLink <= 0 {
		return nil, fmt.Errorf("rss expiration link should be > 0")
	}
	oidc, err := newOidcContext(authConfig, serverPublicUrl+"/oidc/callback", "/")
	if err != nil {
		return nil, fmt.Errorf("cant init oidc: %w", err)
	}
	router := mux.NewRouter()
	srv := &http.Server{
		Addr:    listen,
		Handler: router,
	}
	server := &httpServer{
		server:            srv,
		mux:               router,
		storage:           storage,
		oidc:              oidc,
		serverPublicUrl:   serverPublicUrl,
		rssExpirationLink: rssExpirationLink,
	}

	server.mux.Use(
		func(handler http.Handler) http.Handler {
			return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				defer func() {
					if err := recover(); err != nil {
						log.FromContext(request.Context()).
							With(log.Error(fmt.Errorf("%s", err))).
							Error("PANIC")
						writer.WriteHeader(http.StatusInternalServerError)
					}
				}()

				handler.ServeHTTP(writer, request)
			})
		},
		func(handler http.Handler) http.Handler {
			return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				ctx := request.Context()
				logger := log.FromContext(ctx).
					With(slog.String("request_id", request.Header.Get("x-request-id")))
				ctx = log.PutIntoContext(ctx, logger)
				request = request.WithContext(ctx)
				handler.ServeHTTP(writer, request)
			})
		},
		func(handler http.Handler) http.Handler {
			return handlers.CustomLoggingHandler(io.Discard, handler, func(_ io.Writer, params handlers.LogFormatterParams) {
				log.FromContext(params.Request.Context()).
					With(slog.Int("status_code", params.StatusCode)).
					With(slog.Int("size", params.Size)).
					With(slog.Duration("duration", time.Now().Sub(params.TimeStamp))).
					Info("request processed")
			})
		},
		handlers.CompressHandler,
	)
	server.mux.Name("static").PathPrefix("/static/").Handler(
		restartEtag(
			cacheMiddleware(
				http.FileServer(
					http.FS(
						public.Static,
					),
				),
				5*time.Minute,
			),
		),
	)
	server.mux.Path("/login").HandlerFunc(server.htmxPageLogin)
	server.mux.Path("/oidc/callback").Handler(server.oidc.AuthCallbackHandler())
	server.mux.Path("/oidc/login").Handler(server.oidc.AuthLoginHandler())

	if diagnosticEndpointsEnabled {
		server.mux.Path("/metrics").Handler(promhttp.Handler())
		server.mux.Path("/healthz").HandlerFunc(server.apiPing)
		server.mux.Path("/readyz").HandlerFunc(server.apiPing)
	}

	server.mux.PathPrefix("/rss/").Methods(http.MethodGet).HandlerFunc(server.generateRSS)
	htmx := server.mux.Name("htmx").Subrouter()
	htmx.Use(server.AuthMiddleware())
	htmx.Path("/").HandlerFunc(server.htmxPageMain)
	htmx.Path("/whoami").HandlerFunc(server.htmxPageWhoami)

	api := server.mux.Name("api").PathPrefix("/api/").Subrouter()
	api.Use(server.AuthMiddleware())
	api.Path("/upload").Methods(http.MethodPost).HandlerFunc(server.apiUploadFile)
	api.Path("/delete").Methods(http.MethodDelete).HandlerFunc(server.apiDelteFile)
	api.Path("/link").Methods(http.MethodGet).HandlerFunc(server.apiGenerateDownloadFileLink)

	return server, nil
}
