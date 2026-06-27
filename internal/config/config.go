package config

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

// Config holds all application configuration, loaded from environment
// variables (and an optional .env file for local development).
type Config struct {
	Env         string `mapstructure:"ENV"`
	Port        string `mapstructure:"PORT"`
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	LogLevel    string `mapstructure:"LOG_LEVEL"`

	JWTSecret           string `mapstructure:"JWT_SECRET"`
	JWTAccessTTLMinutes int    `mapstructure:"JWT_ACCESS_TTL_MINUTES"`
	JWTRefreshTTLHours  int    `mapstructure:"JWT_REFRESH_TTL_HOURS"`

	// Payment gateway. The MVP ships a self-contained sandbox provider so the
	// tenant Pay Now flow works without external credentials; real providers
	// replace it via PAYMENT_GATEWAY_PROVIDER.
	PaymentGatewayProvider         string `mapstructure:"PAYMENT_GATEWAY_PROVIDER"`
	PaymentGatewayCheckoutBaseURL  string `mapstructure:"PAYMENT_GATEWAY_CHECKOUT_BASE_URL"`
	PaymentGatewayReturnURL        string `mapstructure:"PAYMENT_GATEWAY_RETURN_URL"`
	PaymentGatewayCheckoutTTLHours int    `mapstructure:"PAYMENT_GATEWAY_CHECKOUT_TTL_HOURS"`
}

// Load reads configuration from the environment, falling back to a local
// .env file and sane defaults.
func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("ENV", "development")
	v.SetDefault("PORT", "8080")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("JWT_ACCESS_TTL_MINUTES", 60)
	v.SetDefault("JWT_REFRESH_TTL_HOURS", 720)
	v.SetDefault("PAYMENT_GATEWAY_PROVIDER", "sandbox")
	v.SetDefault("PAYMENT_GATEWAY_CHECKOUT_BASE_URL", "https://sandbox.pay.local/checkout")
	v.SetDefault("PAYMENT_GATEWAY_RETURN_URL", "https://app.example.com/tenant/payment-result")
	v.SetDefault("PAYMENT_GATEWAY_CHECKOUT_TTL_HOURS", 24)

	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AddConfigPath("./..")

	// Read .env if present; ignore if the file simply doesn't exist.
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return &cfg, nil
}
