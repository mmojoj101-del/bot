package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration.
type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	CORS     CORSConfig
	Log      LogConfig
}

// AppConfig holds general application settings.
type AppConfig struct {
	Name        string
	Environment string
	Version     string
}

// HTTPConfig holds HTTP server settings.
type HTTPConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	GracefulTimeout time.Duration
	TrustedProxies  []string
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	URL             string
	Host            string
	Port            int
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL      string
	Host     string
	Port     int
	Password string
	DB       int
}

// JWTConfig holds JWT settings.
type JWTConfig struct {
	Secret           string
	AccessDuration   time.Duration
	RefreshDuration  time.Duration
	Issuer           string
	Audience         string
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string
	Format string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			Name:        getEnv("APP_NAME", "fury-sms-gateway"),
			Environment: getEnv("APP_ENV", "development"),
			Version:     getEnv("APP_VERSION", "0.1.0"),
		},
		HTTP: HTTPConfig{
			Host:            getEnv("HTTP_HOST", "0.0.0.0"),
			Port:            getEnvInt("HTTP_PORT", 8080),
			ReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			GracefulTimeout: getEnvDuration("HTTP_GRACEFUL_TIMEOUT", 15*time.Second),
			TrustedProxies:  getEnvSlice("HTTP_TRUSTED_PROXIES", []string{}),
		},
		Database: DatabaseConfig{
			URL:             getEnv("DATABASE_URL", ""),
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnvInt("DB_PORT", 5432),
			User:            getEnv("DB_USER", "fury"),
			Password:        getEnv("DB_PASSWORD", "fury"),
			Name:            getEnv("DB_NAME", "fury_sms"),
			SSLMode:         getEnv("DB_SSLMODE", "disable"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", ""),
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:          getEnv("JWT_SECRET", "super-secret-key-change-in-production"),
			AccessDuration:  getEnvDuration("JWT_ACCESS_DURATION", 15*time.Minute),
			RefreshDuration: getEnvDuration("JWT_REFRESH_DURATION", 720*time.Hour), // 30 days
			Issuer:          getEnv("JWT_ISSUER", "fury-sms-gateway"),
			Audience:        getEnv("JWT_AUDIENCE", "fury-api"),
		},
		CORS: CORSConfig{
			AllowedOrigins:   getEnvSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:5173", "http://localhost:3000"}),
			AllowedMethods:   getEnvSlice("CORS_ALLOWED_METHODS", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}),
			AllowedHeaders:   getEnvSlice("CORS_ALLOWED_HEADERS", []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key", "X-Tenant-ID"}),
			AllowCredentials: getEnvBool("CORS_ALLOW_CREDENTIALS", true),
			MaxAge:           getEnvInt("CORS_MAX_AGE", 300),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
	}

	// Build database URL from parts if not set directly
	if cfg.Database.URL == "" {
		cfg.Database.URL = fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s?sslmode=%s",
			cfg.Database.User, cfg.Database.Password,
			cfg.Database.Host, cfg.Database.Port,
			cfg.Database.Name, cfg.Database.SSLMode,
		)
	}

	// Build Redis URL from parts if not set directly
	if cfg.Redis.URL == "" {
		cfg.Redis.URL = fmt.Sprintf(
			"redis://:%s@%s:%d/%d",
			cfg.Redis.Password, cfg.Redis.Host, cfg.Redis.Port, cfg.Redis.DB,
		)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.JWT.Secret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if c.Database.URL == "" {
		return fmt.Errorf("database URL is required")
	}
	return nil
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvInt returns the integer value of an environment variable or a default value.
func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// getEnvBool returns the boolean value of an environment variable or a default value.
func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

// getEnvDuration returns the duration value of an environment variable or a default value.
func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

// getEnvSlice returns the slice value of an environment variable or a default value.
// The environment variable should be comma-separated.
func getEnvSlice(key string, defaultVal []string) []string {
	if val := os.Getenv(key); val != "" {
		result := []string{}
		for _, v := range split(val, ",") {
			if trimmed := trimSpace(v); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultVal
}

// Helper functions to avoid importing strings everywhere in config
func split(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		result = append(result, s[start:])
	}
	return result
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
