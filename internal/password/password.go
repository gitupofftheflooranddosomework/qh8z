package password

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	iterations = 600000
	saltBytes  = 16
	keyBytes   = 32
)

func Hash(raw string) (string, error) {
	if err := Validate(raw); err != nil {
		return "", err
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, err := derive(sha256.New, raw, salt, iterations, keyBytes)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$pbkdf2-sha256$%d$%s$%s", iterations,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func Verify(encoded, raw string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[1] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[2])
	if err != nil || iter < 100000 || iter > 2000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(expected) != keyBytes {
		return false
	}
	actual, err := derive(sha256.New, raw, salt, iter, len(expected))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

func Validate(raw string) error {
	if len(raw) < 12 {
		return errors.New("password must be at least 12 characters")
	}
	if len(raw) > 128 {
		return errors.New("password must be at most 128 bytes")
	}
	return nil
}
