package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/paragor/sharefile/internal/httpserver"
	"github.com/paragor/sharefile/internal/log"
	"github.com/paragor/sharefile/internal/storage"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Listen                     string `yaml:"listen"`
	ServerPublicUrl            string `yaml:"server_public_url"`
	DiagnosticEndpointsEnabled bool   `yaml:"diagnostic_endpoints_enabled"`
	RssExpirationLinkHours     int    `yaml:"rss_expiration_link_hours"`

	Oidc struct {
		ClientId     string   `yaml:"client_id"`
		ClientSecret string   `yaml:"client_secret"`
		IssuerUrl    string   `yaml:"issuer_url"`
		CookieKey    string   `yaml:"cookie_key"`
		Scopes       []string `yaml:"scopes"`
		AllowedGroup string   `yaml:"allowed_group"`
		AdminGroup   string   `yaml:"admin_group"`
	} `yaml:"oidc"`

	Storage struct {
		Type string `yaml:"type"`
		S3   struct {
			Bucket          string `yaml:"bucket"`
			Endpoint        string `yaml:"endpoint"`
			Region          string `yaml:"region"`
			AccessKeyId     string `yaml:"access_key_id"`
			SecretAccessKey string `yaml:"secret_access_key"`
			PathStyle       bool   `yaml:"path_style"`
			DisableSSL      bool   `yaml:"disable_ssl"`
		} `yaml:"s3"`
	} `yaml:"storage"`
}

func main() {
	logger := log.FromContext(context.Background())

	configPath := flag.String("config", "config.yaml", "path to config")
	dumpDefaultConfig := flag.Bool("dump-default-config", false, "dump default config")
	flag.Parse()

	cfg := &Config{}
	cfg.Listen = "127.0.0.1:8080"
	cfg.ServerPublicUrl = "http://127.0.0.1:8080"
	cfg.Oidc.Scopes = []string{"openid", "email", "profile", "offline_access"}
	cfg.Storage.Type = "s3"
	cfg.RssExpirationLinkHours = 1

	if *dumpDefaultConfig {
		cfg.Oidc.CookieKey = "kiel4teof4Eoziheigiesh7ooquiepho"
		if err := yaml.NewEncoder(os.Stdout).Encode(cfg); err != nil {
			logger.With(log.Error(err)).Error("fail to dump default config")
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfgContent, err := os.ReadFile(*configPath)
	if err != nil {
		logger.With(log.Error(err), slog.String("path", *configPath)).Error("fail to read config")
		os.Exit(1)
	}
	if err := yaml.Unmarshal(cfgContent, cfg); err != nil {
		logger.With(log.Error(err), slog.String("path", *configPath)).Error("fail to unmarshal config")
		os.Exit(1)
	}

	var storageInstance storage.Storage
	switch cfg.Storage.Type {
	case "s3":
		storageInstance, err = initS3Storage(cfg)
		if err != nil {
			logger.With(log.Error(err)).Error("fail to init s3 storage")
			os.Exit(1)
		}
	default:
		logger.With(slog.String("type", cfg.Storage.Type)).Error("unsupported storage type")
		os.Exit(1)
	}
	auth := &httpserver.AuthOidcConfig{
		ClientId:     cfg.Oidc.ClientId,
		ClientSecret: cfg.Oidc.ClientSecret,
		IssuerUrl:    cfg.Oidc.IssuerUrl,
		CookieKey:    cfg.Oidc.CookieKey,
		Scopes:       cfg.Oidc.Scopes,
		AllowedGroup: cfg.Oidc.AllowedGroup,
		AdminGroup:   cfg.Oidc.AdminGroup,
	}
	if err := auth.Validate(); err != nil {
		logger.With(log.Error(err)).Error("invalid oauth config")
		os.Exit(1)
	}

	server, err := httpserver.NewHttpServer(
		cfg.Listen,
		storageInstance,
		auth,
		cfg.ServerPublicUrl,
		cfg.DiagnosticEndpointsEnabled,
		time.Hour*time.Duration(cfg.RssExpirationLinkHours),
	)
	if err != nil {
		logger.With(log.Error(err)).Error("fail to start server")
		os.Exit(1)
	}

	mainCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server started!")
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()
	select {
	case <-mainCtx.Done():
		logger.Info("graceful shutdown starts...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer shutdownCancel()
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			logger.With(log.Error(err)).Error("fail to shutdown server")
			os.Exit(1)
		}
	case err := <-serverErrors:
		logger.With(log.Error(err)).Error("fail on start server")
		os.Exit(1)
	}

	logger.Info("graceful shutdown complete!")
}

func initS3Storage(cfg *Config) (storage.Storage, error) {
	if cfg.Storage.S3.Bucket == "" {
		return nil, fmt.Errorf("bucket name in config should not be empty")
	}
	awsConfig := aws.NewConfig().
		WithCredentials(
			credentials.NewStaticCredentials(
				cfg.Storage.S3.AccessKeyId,
				cfg.Storage.S3.SecretAccessKey,
				"",
			),
		).
		WithDisableSSL(cfg.Storage.S3.DisableSSL).
		WithS3ForcePathStyle(cfg.Storage.S3.PathStyle).
		WithRegion(cfg.Storage.S3.Region).
		WithEndpoint(cfg.Storage.S3.Endpoint)

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("fail to init aws session: %w", err)
	}

	s3Client := s3.New(sess, awsConfig)
	return storage.NewS3Storage(
		s3Client,
		cfg.Storage.S3.Bucket,
	), nil
}
