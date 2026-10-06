package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/AndrewDMcC/estate-agency/internal/rightmove"
)

type Config struct {
	Addr          string
	DataDir       string
	BaseURL       string
	AdminUser     string
	AdminPassword string
	SeedDemo      bool
	Rightmove     rightmove.Config
}

func Load() (Config, error) {
	cfg := Config{
		Addr:          env("ADDR", ":8080"),
		DataDir:       env("DATA_DIR", "./data"),
		BaseURL:       strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		AdminUser:     env("ADMIN_USER", "admin"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		SeedDemo:      env("SEED_DEMO", "true") == "true",
		Rightmove: rightmove.Config{
			Mode:     rightmove.Mode(env("RIGHTMOVE_MODE", string(rightmove.ModeDryRun))),
			CertFile: os.Getenv("RIGHTMOVE_CERT_FILE"),
			KeyFile:  os.Getenv("RIGHTMOVE_KEY_FILE"),
			APIBase:  os.Getenv("RIGHTMOVE_API_BASE"),
		},
	}
	var err error
	if cfg.Rightmove.NetworkID, err = envInt("RIGHTMOVE_NETWORK_ID"); err != nil {
		return cfg, err
	}
	if cfg.Rightmove.BranchID, err = envInt("RIGHTMOVE_BRANCH_ID"); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", key, err)
	}
	return n, nil
}
