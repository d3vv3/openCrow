package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/opencrow/opencrow/server/internal/api"
	"github.com/opencrow/opencrow/server/internal/app"
	"github.com/opencrow/opencrow/server/internal/auth"
	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/storage"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := storage.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("init postgres: %v", err)
	}
	defer db.Close()

	redisClient, err := storage.NewRedisClient(ctx, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("init redis: %v", err)
	}
	defer redisClient.Close()

	_ = redisClient

	authMgr := auth.NewManager(cfg.JWTIssuer, cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	cfgStore, err := configstore.New(cfg.ConfigFilePath)
	if err != nil {
		log.Fatalf("init config store: %v", err)
	}

	server := api.NewServer(cfg.Env, db, authMgr, cfgStore, api.Options{
		AdminUsername:       cfg.AdminUsername,
		AdminPasswordBcrypt: cfg.AdminPasswordBcrypt,
		ServerShellTimeout:  cfg.ServerShellTimeout,
		StateDir:            cfg.StateDir,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           withCORS(server.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("openCrow server listening on %s", cfg.Addr())
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen and serve: %v", err)
		}
	}()

	server.StartWorkers(ctx)

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Client-Timezone")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
