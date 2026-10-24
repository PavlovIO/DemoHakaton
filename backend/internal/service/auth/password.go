package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

const (
	passwordMemory      = 64 * 1024
	passwordIterations  = 3
	passwordParallelism = 4
	passwordSaltLength  = 16
	passwordKeyLength   = 32
	maxParseMemory      = 256 * 1024
	maxParseIterations  = 10
	maxParseParallelism = 16
	maxConcurrentHashes = 4
)

const (
	// MinPasswordLength — минимальная длина пароля в символах.
	MinPasswordLength = 10
	// MaxPasswordLength — максимальная длина пароля в символах.
	MaxPasswordLength = 128
)

var hashSlots = make(chan struct{}, maxConcurrentHashes)

// HashPassword хэширует NFC-нормализованный пароль алгоритмом Argon2id в формате PHC.
func HashPassword(ctx context.Context, plain string) (string, error) {
	if err := acquireHashSlot(ctx); err != nil {
		return "", err
	}
	defer releaseHashSlot()

	salt := make([]byte, passwordSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	hash := argon2.IDKey([]byte(norm.NFC.String(plain)), salt, passwordIterations, passwordMemory, passwordParallelism, passwordKeyLength)
	encoding := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		passwordMemory,
		passwordIterations,
		passwordParallelism,
		encoding.EncodeToString(salt),
		encoding.EncodeToString(hash),
	), nil
}

// VerifyPassword сверяет NFC-нормализованный пароль с Argon2id-хэшем в формате PHC.
func VerifyPassword(ctx context.Context, plain, encoded string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	params, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, err
	}

	if err := acquireHashSlot(ctx); err != nil {
		return false, err
	}
	defer releaseHashSlot()

	actual := argon2.IDKey([]byte(norm.NFC.String(plain)), salt, params.iterations, params.memory, params.parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

// ValidatePassword проверяет NFC-нормализованную длину пароля в рунах.
func ValidatePassword(plain string) error {
	length := utf8.RuneCountInString(norm.NFC.String(plain))

	if length < MinPasswordLength {
		return apperr.Validation(
			fmt.Sprintf("Пароль должен содержать не менее %d символов", MinPasswordLength),
			apperr.FieldError{Field: "password", Message: fmt.Sprintf("минимум %d символов", MinPasswordLength)},
		)
	}

	if length > MaxPasswordLength {
		return apperr.Validation(
			fmt.Sprintf("Пароль должен содержать не более %d символов", MaxPasswordLength),
			apperr.FieldError{Field: "password", Message: fmt.Sprintf("максимум %d символов", MaxPasswordLength)},
		)
	}

	return nil
}

func acquireHashSlot(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case hashSlots <- struct{}{}:
	}
	if err := ctx.Err(); err != nil {
		releaseHashSlot()
		return err
	}
	return nil
}

func releaseHashSlot() {
	<-hashSlots
}

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parsePasswordHash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id password hash format")
	}

	params, err := parseArgon2Params(parts[3])
	if err != nil {
		return argon2Params{}, nil, nil, err
	}

	encoding := base64.RawStdEncoding.Strict()
	salt, err := encoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id password hash salt")
	}
	hash, err := encoding.DecodeString(parts[5])
	if err != nil || len(hash) < 16 || len(hash) > 64 {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid Argon2id password hash key")
	}
	return params, salt, hash, nil
}

func parseArgon2Params(encoded string) (argon2Params, error) {
	values := make(map[string]uint64, 3)
	for _, field := range strings.Split(encoded, ",") {
		key, value, ok := strings.Cut(field, "=")
		if !ok || (key != "m" && key != "t" && key != "p") {
			return argon2Params{}, fmt.Errorf("invalid Argon2id password hash parameters")
		}
		if _, exists := values[key]; exists {
			return argon2Params{}, fmt.Errorf("duplicate Argon2id password hash parameter %q", key)
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return argon2Params{}, fmt.Errorf("invalid Argon2id password hash parameter %q: %w", key, err)
		}
		values[key] = parsed
	}

	memory, hasMemory := values["m"]
	iterations, hasIterations := values["t"]
	parallelism, hasParallelism := values["p"]
	if !hasMemory || !hasIterations || !hasParallelism ||
		memory < 8*parallelism || memory > maxParseMemory ||
		iterations == 0 || iterations > maxParseIterations ||
		parallelism == 0 || parallelism > maxParseParallelism {
		return argon2Params{}, fmt.Errorf("unsupported Argon2id password hash parameters")
	}
	return argon2Params{
		memory:      uint32(memory),
		iterations:  uint32(iterations),
		parallelism: uint8(parallelism),
	}, nil
}
