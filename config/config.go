// config/config.go
package config

import (
	"os"
)

// Config contains application configuration parameters
type Config struct {
	Port            string `json:"port"`
	Token           string `json:"token"`
	BaseURL         string `json:"base_url"`
	DBName          string `json:"db_name"`
	SavePaymentsDir string `json"save_payments_dir"`
}

// NewConfig creates and returns a new configuration instance
func NewConfig() (*Config, error) {
	cfg := &Config{
		Port:            ":8080",
		Token:           "7236771363:AAHC7J1nUx1o_OmQYhk1PVl2eRSwp-zouo4",
		BaseURL:         "https://yourdomain.com", // Update this with your actual domain
		DBName:          "meily.db",
		SavePaymentsDir: "./payment",
	}

	// Override with environment variables if set
	if port := os.Getenv("PORT"); port != "" {
		cfg.Port = ":" + port
	}

	if token := os.Getenv("BOT_TOKEN"); token != "" {
		cfg.Token = token
	}

	if baseURL := os.Getenv("BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}

	if dbName := os.Getenv("DB_NAME"); dbName != "" {
		cfg.DBName = dbName
	}

	if savePaymentsDir := os.Getenv("SAVE_PAYMENTS_DIR"); savePaymentsDir != "" {
		cfg.DBName = savePaymentsDir
	}

	return cfg, nil
}
