package app

import (
	"errors"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/config"
	"github.com/refa-optical/backend/internal/database"
	"github.com/refa-optical/backend/internal/handlers"
	"github.com/refa-optical/backend/internal/payment"
	"github.com/refa-optical/backend/internal/router"
	"github.com/refa-optical/backend/internal/storage"
	"gorm.io/gorm"
)

// New initializes the API dependencies and returns the HTTP handler used by
// both the local server and Vercel's serverless function runtime.
func New(cfg config.Config) (http.Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.GinMode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	var db *gorm.DB
	var err error
	if os.Getenv("VERCEL") == "1" {
		// The production schema already exists; migrations and catalog seeding
		// are intentionally kept out of Vercel function startup.
		db, err = database.Connect(cfg.DatabaseDSN, cfg.DatabaseCAPath)
	} else {
		db, err = database.Open(cfg.DatabaseDSN, cfg.DatabaseCAPath)
	}
	if err != nil {
		return nil, err
	}
	if os.Getenv("VERCEL") != "1" {
		err = database.SeedCatalog(db)
	}
	if err != nil {
		return nil, err
	}
	store, err := buildStorage(cfg)
	if err != nil {
		return nil, err
	}
	api := &handlers.Server{DB: db, Config: cfg, Store: store, Gateway: payment.NewMidtrans(cfg.MidtransServerKey, cfg.MidtransProduction)}
	if err = api.CreateAdminIfConfigured(cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return nil, err
	}
	return router.New(cfg, api), nil
}

func buildStorage(cfg config.Config) (storage.Store, error) {
	switch cfg.StorageDriver {
	case "local":
		root := cfg.UploadDir
		if os.Getenv("VERCEL") == "1" {
			// Vercel's deployment filesystem is read-only; /tmp is writable but
			// ephemeral. Configure S3-compatible storage for persistent uploads.
			root = "/tmp/refa-optical/uploads/private"
		}
		return storage.NewLocal(root)
	case "s3":
		return storage.NewS3(cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	default:
		return nil, errors.New("STORAGE_DRIVER must be local or s3")
	}
}
