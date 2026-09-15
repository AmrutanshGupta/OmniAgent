package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	EncryptionKey       string
	NextAuthSecret      string
	MongoDBURI          string
	MongoDBName         string
	PythonRouterURL     string
	PistonAPIURL        string
	Port                string
	PistonTimeoutMs     int
	PistonMaxConcurrent int
	CoderMaxAttempts    int
	SandboxKeywords     string
	EscalationTierOrder string
	CronSchedule        time.Duration
	EmaAlpha            float64
	WorkspaceRoot       string
}

func Load() (*Config, error) {
	cfg := &Config{
		EncryptionKey:       os.Getenv("ENCRYPTION_KEY"),
		NextAuthSecret:      os.Getenv("NEXTAUTH_SECRET"),
		MongoDBURI:          os.Getenv("MONGODB_URI"),
		MongoDBName:         os.Getenv("MONGODB_DB"),
		PythonRouterURL:     os.Getenv("PYTHON_ROUTER_URL"),
		PistonAPIURL:        os.Getenv("PISTON_API_URL"),
		Port:                os.Getenv("PORT"),
		SandboxKeywords:     os.Getenv("SANDBOX_KEYWORDS"),
		EscalationTierOrder: os.Getenv("ESCALATION_TIER_ORDER"),
		WorkspaceRoot:       os.Getenv("WORKSPACE_ROOT"),
	}

	// Validation
	if len(cfg.EncryptionKey) != 32 {
		return nil, errors.New("ENCRYPTION_KEY must be exactly 32 bytes for AES-256")
	}
	if cfg.NextAuthSecret == "" {
		return nil, errors.New("NEXTAUTH_SECRET is required")
	}

	// Defaults & Fallbacks
	if cfg.MongoDBURI == "" {
		// Try fallback MONGO_URI
		fallback := os.Getenv("MONGO_URI")
		if fallback != "" {
			cfg.MongoDBURI = fallback
		} else {
			cfg.MongoDBURI = "mongodb://localhost:27017"
		}
	}
	if cfg.MongoDBName == "" {
		cfg.MongoDBName = "test"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.PythonRouterURL == "" {
		cfg.PythonRouterURL = "http://localhost:8000"
	}
	if cfg.PistonAPIURL == "" {
		cfg.PistonAPIURL = "http://localhost:2000"
	}

	// Integers
	if timeout := os.Getenv("PISTON_TIMEOUT_MS"); timeout != "" {
		v, err := strconv.Atoi(timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid PISTON_TIMEOUT_MS: %w", err)
		}
		cfg.PistonTimeoutMs = v
	} else {
		cfg.PistonTimeoutMs = 5000 // Default 5s
	}

	if maxc := os.Getenv("PISTON_MAX_CONCURRENT"); maxc != "" {
		v, err := strconv.Atoi(maxc)
		if err != nil {
			return nil, fmt.Errorf("invalid PISTON_MAX_CONCURRENT: %w", err)
		}
		cfg.PistonMaxConcurrent = v
	} else {
		cfg.PistonMaxConcurrent = 10
	}

	if attempts := os.Getenv("CODER_MAX_ATTEMPTS"); attempts != "" {
		v, err := strconv.Atoi(attempts)
		if err != nil {
			return nil, fmt.Errorf("invalid CODER_MAX_ATTEMPTS: %w", err)
		}
		cfg.CoderMaxAttempts = v
	} else {
		cfg.CoderMaxAttempts = 3
	}

	// Floats
	if alpha := os.Getenv("EMA_ALPHA"); alpha != "" {
		v, err := strconv.ParseFloat(alpha, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid EMA_ALPHA: %w", err)
		}
		cfg.EmaAlpha = v
	} else {
		cfg.EmaAlpha = 0.1
	}

	// Duration
	cfg.CronSchedule = 6 * time.Hour
	if cronEnv := os.Getenv("CRON_SCHEDULE"); cronEnv != "" {
		if parsed, err := time.ParseDuration(cronEnv); err == nil {
			cfg.CronSchedule = parsed
		}
	}

	return cfg, nil
}
