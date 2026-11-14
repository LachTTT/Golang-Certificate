package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	JWTSecret     string
	PaymentTTL    time.Duration
	CoreURL       string
	InternalToken string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func Load() Config {
	return Config{
		Port:          getenv("PORT", "8082"),
		DBHost:        getenv("DB_HOST", "localhost"),
		DBPort:        getenv("DB_PORT", "5432"),
		DBUser:        getenv("DB_USER", "postgres"),
		DBPassword:    os.Getenv("DB_PASSWORD"),
		DBName:        getenv("PAYMENT_DB_NAME", "ewallet_payment"),
		DBSSLMode:     getenv("DB_SSLMODE", "disable"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		PaymentTTL:    time.Duration(getenvInt("PAYMENT_TTL_MINUTES", 10)) * time.Minute,
		CoreURL:       getenv("CORE_SERVICE_URL", "http://localhost:8081"),
		InternalToken: os.Getenv("INTERNAL_TOKEN"),
	}
}

func (c Config) Validate() error {
	if c.DBPassword == "" {
		return errors.New("DB_PASSWORD is required")
	}
	if len(c.JWTSecret) < 16 {
		return errors.New("JWT_SECRET is required (min 16 chars)")
	}
	if len(c.InternalToken) < 16 {
		return errors.New("INTERNAL_TOKEN is required (min 16 chars)")
	}
	return nil
}

func (c Config) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     net.JoinHostPort(c.DBHost, c.DBPort),
		Path:     "/" + c.DBName,
		RawQuery: "sslmode=" + c.DBSSLMode,
	}
	return u.String()
}
