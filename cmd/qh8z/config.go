package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

func secretValue(key string) (string, error) {
	if path := strings.TrimSpace(os.Getenv(key + "_FILE")); path != "" {
		value, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s_FILE: %w", key, err)
		}
		return strings.TrimSpace(string(value)), nil
	}
	return strings.TrimSpace(os.Getenv(key)), nil
}

func databaseURL() (string, error) {
	if dsn, err := secretValue("DATABASE_URL"); err != nil || dsn != "" {
		return dsn, err
	}
	password, err := secretValue("DATABASE_PASSWORD")
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", errors.New("DATABASE_URL or DATABASE_PASSWORD is required for postgres storage")
	}
	host := envOr("DATABASE_HOST", "postgres")
	port := envOr("DATABASE_PORT", "5432")
	user := envOr("DATABASE_USER", "qh8z")
	name := envOr("DATABASE_NAME", "qh8z")
	sslmode := envOr("DATABASE_SSLMODE", "disable")
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   host + ":" + port,
		Path:   "/" + name,
	}
	query := u.Query()
	query.Set("sslmode", sslmode)
	u.RawQuery = query.Encode()
	return u.String(), nil
}
