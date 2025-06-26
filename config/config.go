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
	SavePaymentsDir string `json:"save_payments_dir"`
	AdminID         int64  `json:"admin_id"`
	StartPhotoId    string `json:"start_photo_id"`
	StartVideoId    string `json:"start_video_id"`
	Cost            int    `json:"cost"`
	BotUsername     string `json:"bot_username"`
}

// NewConfig creates and returns a new configuration instance
func NewConfig() (*Config, error) {
	cfg := &Config{
		Port:            ":8080",
		Token:           "7236771363:AAHC7J1nUx1o_OmQYhk1PVl2eRSwp-zouo4",
		BaseURL:         "https://ccc8-89-219-13-135.ngrok-free.app", // Update this with your actual domain
		DBName:          "meily.db",
		SavePaymentsDir: "./payment",
		AdminID:         800703982,
		StartPhotoId:    "AgACAgIAAxkBAANSaFP5emhGuJ5qTUamzTYon-yyPv4AAszxMRuxzqBKW2jULQVc0e4BAAMCAAN5AAM2BA",
		StartVideoId:    "",
		Cost:            18900,
		BotUsername:     "meilly_cosmetics_bot",
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
