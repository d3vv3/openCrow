package app

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env string

	APIHost string
	APIPort string

	DatabaseURL string

	JWTIssuer     string
	JWTSecret     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration

	StateDir        string
	ConfigFilePath  string
	WhisperModel    string
	WhisperEndpoint string
	KokoroEndpoint  string

	AdminUsername       string
	AdminPasswordBcrypt string
	ServerShellTimeout  time.Duration
}

func LoadConfig() (Config, error) {
	accessTTL, err := parseDurationWithDefault("JWT_ACCESS_TTL", "15m")
	if err != nil {
		return Config{}, fmt.Errorf("invalid JWT_ACCESS_TTL: %w", err)
	}

	refreshTTL, err := parseDurationWithDefault("JWT_REFRESH_TTL", "720h")
	if err != nil {
		return Config{}, fmt.Errorf("invalid JWT_REFRESH_TTL: %w", err)
	}

	shellTimeout, err := parseDurationWithDefault("SERVER_SHELL_TIMEOUT", "300s")
	if err != nil {
		return Config{}, fmt.Errorf("invalid SERVER_SHELL_TIMEOUT: %w", err)
	}

	cfg := Config{
		Env:                 getEnv("APP_ENV", "development"),
		APIHost:             getEnv("API_HOST", "0.0.0.0"),
		APIPort:             getEnv("API_PORT", "8080"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		JWTIssuer:           getEnv("JWT_ISSUER", "openCrow"),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		JWTAccessTTL:        accessTTL,
		JWTRefreshTTL:       refreshTTL,
		StateDir:            getEnv("STATE_DIR", "/data"),
		WhisperModel:        getEnv("WHISPER_MODEL", "ggml-base"),
		WhisperEndpoint:     os.Getenv("WHISPER_ENDPOINT"),
		KokoroEndpoint:      os.Getenv("KOKORO_ENDPOINT"),
		AdminUsername:       strings.TrimSpace(os.Getenv("ADMIN_USERNAME")),
		AdminPasswordBcrypt: strings.TrimSpace(os.Getenv("ADMIN_PASSWORD_BCRYPT")),
		ServerShellTimeout:  shellTimeout,
	}
	cfg.ConfigFilePath = getEnv("CONFIG_FILE_PATH", cfg.StateDir+"/config.json")

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.AdminUsername == "" || cfg.AdminPasswordBcrypt == "" {
		return Config{}, fmt.Errorf("single-user mode requires ADMIN_USERNAME and ADMIN_PASSWORD_BCRYPT")
	}

	return cfg, nil
}

func (c Config) Addr() string {
	return c.APIHost + ":" + c.APIPort
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseDurationWithDefault(key, fallback string) (time.Duration, error) {
	value := getEnv(key, fallback)
	return time.ParseDuration(value)
}

func parseBoolWithDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
