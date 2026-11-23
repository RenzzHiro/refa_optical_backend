package main

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/config"
	"github.com/refa-optical/backend/internal/database"
	"github.com/refa-optical/backend/internal/handlers"
	"github.com/refa-optical/backend/internal/payment"
	"github.com/refa-optical/backend/internal/router"
	"github.com/refa-optical/backend/internal/storage"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	if cfg.GinMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	db, err := database.Open(cfg.DatabaseDSN, cfg.DatabaseCAPath)
	if err != nil {
		log.Fatalf("database startup: %v", err)
	}
	if err = database.SeedCatalog(db); err != nil {
		log.Fatalf("catalog seed: %v", err)
	}
	store, err := buildStorage(cfg)
	if err != nil {
		log.Fatalf("storage startup: %v", err)
	}
	api := &handlers.Server{DB: db, Config: cfg, Store: store, Gateway: payment.NewMidtrans(cfg.MidtransServerKey, cfg.MidtransProduction)}
	if err = api.CreateAdminIfConfigured(cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatalf("admin bootstrap: %v", err)
	}
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router.New(cfg, api), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("Refa Optical API listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown: %v", err)
	}
}
func buildStorage(cfg config.Config) (storage.Store, error) {
	switch cfg.StorageDriver {
	case "local":
		return storage.NewLocal(cfg.UploadDir)
	case "s3":
		return storage.NewS3(cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	default:
		return nil, errors.New("STORAGE_DRIVER must be local or s3")
	}
}
