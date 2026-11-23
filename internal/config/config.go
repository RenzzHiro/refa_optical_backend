package config

import (
	"github.com/joho/godotenv"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	GinMode            string
	DatabaseDSN        string
	DatabaseCAPath     string
	JWTSecret          string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	CookieSecure       bool
	AllowedOrigins     []string
	UploadDir          string
	MaxUploadBytes     int64
	StorageDriver      string
	S3Endpoint         string
	S3Region           string
	S3AccessKey        string
	S3SecretKey        string
	S3Bucket           string
	S3UseSSL           bool
	MidtransServerKey  string
	MidtransProduction bool
	FrontendURL        string
	AdminEmail         string
	AdminPassword      string
}

func Load() Config {
	_ = godotenv.Load()
	ttl, _ := strconv.Atoi(get("ACCESS_TOKEN_TTL_MINUTES", "15"))
	refreshDays, _ := strconv.Atoi(get("REFRESH_TOKEN_TTL_DAYS", "30"))
	maxUpload, _ := strconv.ParseInt(get("MAX_PRESCRIPTION_BYTES", "5242880"), 10, 64)
	return Config{
		Port: get("PORT", "8080"), GinMode: get("GIN_MODE", "debug"),
		DatabaseDSN: get("DATABASE_DSN", "root:@tcp(127.0.0.1:3306)/refa_optical?charset=utf8mb4&parseTime=True&loc=Local"), DatabaseCAPath: get("DATABASE_CA_CERT_PATH", ""),
		JWTSecret:   get("JWT_SECRET", ""), AccessTokenTTL: time.Duration(ttl) * time.Minute, RefreshTokenTTL: time.Duration(refreshDays) * 24 * time.Hour,
		CookieSecure: get("COOKIE_SECURE", "false") == "true", AllowedOrigins: split(get("CORS_ORIGINS", "http://localhost:5173")),
		UploadDir: get("UPLOAD_DIR", "./uploads/private"), MaxUploadBytes: maxUpload, StorageDriver: get("STORAGE_DRIVER", "local"),
		S3Endpoint: get("S3_ENDPOINT", ""), S3Region: get("S3_REGION", "us-east-1"), S3AccessKey: get("S3_ACCESS_KEY", ""), S3SecretKey: get("S3_SECRET_KEY", ""), S3Bucket: get("S3_BUCKET", "refa-prescriptions"), S3UseSSL: get("S3_USE_SSL", "true") == "true",
		MidtransServerKey: get("MIDTRANS_SERVER_KEY", ""), MidtransProduction: get("MIDTRANS_IS_PRODUCTION", "false") == "true", FrontendURL: get("FRONTEND_URL", "http://localhost:5173"), AdminEmail: get("ADMIN_EMAIL", ""), AdminPassword: get("ADMIN_PASSWORD", ""),
	}
}
func (c Config) Validate() error {
	if len(c.JWTSecret) < 32 {
		return ErrJWTSecret
	}
	if c.AccessTokenTTL <= 0 || c.RefreshTokenTTL <= 0 || c.MaxUploadBytes <= 0 {
		return ErrInvalidConfig
	}
	return nil
}
func get(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
func split(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

