package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"trotamundos/internal/agencies"
	"trotamundos/internal/analytics"
	"trotamundos/internal/api"
	"trotamundos/internal/auth"
	"trotamundos/internal/config"
	"trotamundos/internal/ingestion"
	"trotamundos/internal/itinerary"
	"trotamundos/internal/mailer"
	"trotamundos/internal/platform/jwtx"
	"trotamundos/internal/platform/password"
	"trotamundos/internal/platform/pgxdb"
	"trotamundos/internal/platform/redisx"
	"trotamundos/internal/platform/secrets"
	"trotamundos/internal/providers"
	"trotamundos/internal/sharing"
	"trotamundos/internal/social"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}


// api
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg.Env)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shards, err := pgxdb.NewShards(ctx, cfg.DatabaseShards)
	if err != nil {
		return err
	}
	defer shards.Close()
	if err := shards.Migrate(ctx); err != nil {
		return err
	}
	directory := shards.Directory()

	olap, err := pgxdb.OpenReadOnly(ctx, cfg.OLAPDatabaseURL, 10)
	if err != nil {
		return err
	}
	defer olap.Close()

	redisClient := redisx.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer redisClient.Close()
	if err := redisClient.Ping(ctx); err != nil {
		return err
	}

	priv, _, err := jwtx.LoadOrGenerate(cfg.JWTPrivateKeyPath, cfg.JWTPublicKeyPath)
	if err != nil {
		return err
	}
	signer := jwtx.NewSigner(priv, cfg.JWTIssuer, cfg.JWTAccessTTL)
	hasher := password.New(12)
	refreshStore := auth.NewRedisRefreshStore(redisClient)
	authSvc := auth.NewService(auth.NewPostgresUsers(directory), refreshStore, hasher, signer, cfg.RefreshTTL)

	if cfg.SeedAnalystEmail != "" && cfg.SeedAnalystPassword != "" {
		if _, err := authSvc.EnsureUser(ctx, cfg.SeedAnalystEmail, "Analista Trotamundos", cfg.SeedAnalystPassword, "analyst"); err != nil {
			return err
		}
		logger.Info("analyst user ensured", "email", cfg.SeedAnalystEmail)
	}

	box, err := secrets.New(cfg.TokenEncryptionKey)
	if err != nil {
		return err
	}

	analyticsRepo := analytics.NewPostgresRepository(directory, olap)
	analyticsSvc := analytics.NewService(analyticsRepo)

	providerStore := providers.NewMockStore()
	providerRegistry := providers.NewMockRegistry(providerStore)

	itinRepo := itinerary.NewPostgresRepository(directory)
	itinSvc := itinerary.NewService(itinRepo, providerRegistry, analyticsSvc)

	mailConnector := mailer.NewMockConnector()
	ingestionRepo := ingestion.NewPostgresRepository(directory)
	ingestionSvc := ingestion.NewService(ingestionRepo, itinSvc, mailConnector, box, logger)
	ingestionWorker := ingestion.NewWorker(ingestionSvc, ingestionRepo, logger, cfg.IngestionPollInterval, cfg.StaleEventAfter)

	agenciesSvc := agencies.NewService(agencies.NewPostgresRepository(directory), itinSvc)
	sharingSvc := sharing.NewService(sharing.NewPostgresRepository(directory), itinSvc)
	socialSvc := social.NewService(itinSvc)

	router := api.NewRouter(api.Deps{
		Authenticator: auth.NewAuthenticator(signer),
		Auth:          auth.NewHandler(authSvc),
		Itinerary:     itinerary.NewHandler(itinSvc),
		Ingestion:     ingestion.NewHandler(ingestionSvc, mailConnector, cfg.WebhookKey),
		Agencies:      agencies.NewHandler(agenciesSvc),
		Sharing:       sharing.NewHandler(sharingSvc),
		Social:        social.NewHandler(socialSvc),
		Analytics:     analytics.NewHandler(analyticsSvc),
		Providers:     providers.NewHandler(providerStore),
		MockKey:       cfg.MockControlKey,
		Logger:        logger,
		AllowedOrigin: cfg.AllowedOrigin,
		Health: func(r *http.Request) error {
			hctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			for _, p := range shards.Pools() {
				if err := p.Ping(hctx); err != nil {
					return err
				}
			}
			return redisClient.Ping(hctx)
		},
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if cfg.EnableWorkers {
		go ingestionWorker.Run(ctx)
		go syncLoop(ctx, logger, cfg.ProviderSyncInterval, func() {
			ok, failed := itinSvc.SyncDueReservations(ctx, 50)
			if ok+failed > 0 {
				logger.Info("provider sync", "synced", ok, "failed", failed)
			}
		})
		go syncLoop(ctx, logger, cfg.AutoArchiveInterval, func() {
			if n, err := itinSvc.ArchivePastTrips(ctx); err != nil {
				logger.Error("archive past trips", "error", err)
			} else if n > 0 {
				logger.Info("trips archived", "count", n)
			}
		})
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func syncLoop(ctx context.Context, log *slog.Logger, interval time.Duration, fn func()) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn()
		}
	}
}

func newLogger(env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if env == "development" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
