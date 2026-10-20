package service_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

func TestPasswordHashAndVerify(t *testing.T) {
	const password = "correct horse battery staple"
	encoded, err := service.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=1,p=4$") {
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
			got, err := service.VerifyPassword(tt.password, tt.encoded)
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
	first, err := service.HashPassword("same password")
	if err != nil {
		t.Fatalf("first HashPassword() error = %v", err)
	}
	second, err := service.HashPassword("same password")
	if err != nil {
		t.Fatalf("second HashPassword() error = %v", err)
	}
	if first == second {
		t.Fatal("HashPassword() returned the same hash twice")
	}
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.ValidatePassword(tt.plain)
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
