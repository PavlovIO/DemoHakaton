package auth_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
	"golang.org/x/crypto/argon2"
)

func TestPasswordHashAndVerify(t *testing.T) {
	const password = "correct horse battery staple"
	encoded, err := auth.HashPassword(context.Background(), password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("HashPassword() = %q, want the expected Argon2id PHC prefix", encoded)
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || strings.HasSuffix(parts[4], "=") || strings.HasSuffix(parts[5], "=") {
		t.Fatalf("HashPassword() = %q, want unpadded base64 components", encoded)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		t.Fatalf("decode password salt: %v", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		t.Fatalf("decode password hash: %v", err)
	}
	if len(salt) != 16 || len(hash) != 32 {
		t.Errorf("decoded salt/hash lengths = %d/%d, want 16/32", len(salt), len(hash))
	}

	tests := []struct {
		name     string
		password string
		encoded  string
		want     bool
		wantErr  bool
	}{
		{name: "match", password: password, encoded: encoded, want: true},
		{name: "wrong password", password: "different password", encoded: encoded, want: false},
		{name: "empty string", password: password, encoded: "", wantErr: true},
		{name: "wrong algorithm", password: password, encoded: "$scrypt$v=19$m=65536,t=1,p=4$YWJjZGVmZ2hpamts$YWJjZGVmZ2hpamts", wantErr: true},
		{name: "garbage", password: password, encoded: "not a PHC string", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := auth.VerifyPassword(context.Background(), tt.password, tt.encoded)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifyPassword() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("VerifyPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHashPasswordUsesRandomSalt(t *testing.T) {
	first, err := auth.HashPassword(context.Background(), "same password")
	if err != nil {
		t.Fatalf("first HashPassword() error = %v", err)
	}
	second, err := auth.HashPassword(context.Background(), "same password")
	if err != nil {
		t.Fatalf("second HashPassword() error = %v", err)
	}
	if first == second {
		t.Fatal("HashPassword() returned the same hash twice")
	}
}

func TestPasswordFunctionsRespectCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := auth.HashPassword(ctx, "some password"); !errors.Is(err, context.Canceled) {
		t.Errorf("HashPassword() error = %v, want context.Canceled", err)
	}
	if got, err := auth.VerifyPassword(ctx, "some password", "invalid"); got || !errors.Is(err, context.Canceled) {
		t.Errorf("VerifyPassword() = (%v, %v), want (false, context.Canceled)", got, err)
	}
}

func TestPasswordHashNormalizesNFC(t *testing.T) {
	composed, err := auth.HashPassword(context.Background(), "й")
	if err != nil {
		t.Fatalf("HashPassword(composed) error = %v", err)
	}
	decomposed, err := auth.HashPassword(context.Background(), "и\u0306")
	if err != nil {
		t.Fatalf("HashPassword(decomposed) error = %v", err)
	}

	for _, tt := range []struct {
		name    string
		plain   string
		encoded string
	}{
		{name: "decomposed password verifies composed hash", plain: "и\u0306", encoded: composed},
		{name: "composed password verifies decomposed hash", plain: "й", encoded: decomposed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := auth.VerifyPassword(context.Background(), tt.plain, tt.encoded)
			if err != nil || !got {
				t.Fatalf("VerifyPassword() = (%v, %v), want (true, nil)", got, err)
			}
		})
	}
}

func TestVerifyPasswordParseParameterLimits(t *testing.T) {
	const password = "password used for parameter test"
	const memory = 131072
	salt := []byte("0123456789abcdef")
	hash := argon2.IDKey([]byte(password), salt, 1, memory, 4, 16)
	encoded := makePHC(memory, 1, 4, salt, hash)

	got, err := auth.VerifyPassword(context.Background(), password, encoded)
	if err != nil || !got {
		t.Fatalf("VerifyPassword() for m=131072 = (%v, %v), want (true, nil)", got, err)
	}

	overLimit := makePHC(256*1024+1, 1, 4, salt, hash)
	if got, err := auth.VerifyPassword(context.Background(), password, overLimit); got || err == nil {
		t.Errorf("VerifyPassword() for memory above parse ceiling = (%v, %v), want (false, error)", got, err)
	}
}

func TestVerifyPasswordRejectsShortKeys(t *testing.T) {
	salt := []byte("0123456789abcdef")
	for _, keyLength := range []int{1, 15} {
		t.Run(fmt.Sprintf("%d bytes", keyLength), func(t *testing.T) {
			encoded := makePHC(65536, 1, 4, salt, make([]byte, keyLength))
			if got, err := auth.VerifyPassword(context.Background(), "password", encoded); got || err == nil {
				t.Errorf("VerifyPassword() = (%v, %v), want (false, error)", got, err)
			}
		})
	}
}

func makePHC(memory, iterations, parallelism int, salt, hash []byte) string {
	encoding := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory,
		iterations,
		parallelism,
		encoding.EncodeToString(salt),
		encoding.EncodeToString(hash),
	)
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		plain   string
		wantErr bool
	}{
		{name: "nine ASCII characters", plain: "123456789", wantErr: true},
		{name: "ten ASCII characters", plain: "1234567890"},
		{name: "nine Cyrillic characters", plain: "парольный", wantErr: true},
		{name: "ten Cyrillic characters", plain: "парольныйа"},
		{name: "128 characters", plain: strings.Repeat("a", 128)},
		{name: "129 characters", plain: strings.Repeat("a", 129), wantErr: true},
		{name: "NFC-normalized length below minimum", plain: strings.Repeat("a", 8) + "и\u0306", wantErr: true},
		{name: "NFC-normalized length at minimum", plain: strings.Repeat("a", 9) + "и\u0306"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := auth.ValidatePassword(tt.plain)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				appErr, ok := apperr.As(err)
				if !ok {
					t.Fatalf("ValidatePassword() error type = %T, want *apperr.Error", err)
				}
				if appErr.Code != apperr.CodeValidation {
					t.Errorf("error code = %q, want %q", appErr.Code, apperr.CodeValidation)
				}
				if len(appErr.Details) != 1 || appErr.Details[0].Field != "password" {
					t.Errorf("error details = %#v, want a password field error", appErr.Details)
				}
			}
		})
	}
}
