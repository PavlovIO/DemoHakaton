package kyc

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
)

func TestReceiveFile(t *testing.T) {
	const limit = 1 << 10 // маленький лимит вместо 10 МБ: граница проверяется так же
	pdf := []byte("%PDF-1.4\n" + strings.Repeat("x", 600))
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 100)...)
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{2}, 100)...)
	tests := []struct {
		name     string
		content  []byte
		wantMime string
		wantCode apperr.Code // "" — успех
	}{
		{"pdf", pdf, "application/pdf", ""},
		{"png", png, "image/png", ""},
		{"jpeg", jpeg, "image/jpeg", ""},
		{"короче 512 байт с верной сигнатурой", []byte("%PDF-1.7"), "application/pdf", ""},
		{"ровно лимит", append([]byte("%PDF-"), bytes.Repeat([]byte{'a'}, limit-5)...), "application/pdf", ""},
		{"лимит + 1", append([]byte("%PDF-"), bytes.Repeat([]byte{'a'}, limit-4)...), "", apperr.CodeFileTooLarge},
		{"gif", []byte("GIF89a......"), "", apperr.CodeValidation},
		{"zip", []byte("PK\x03\x04......"), "", apperr.CodeValidation},
		{"exe", []byte("MZ\x90\x00......"), "", apperr.CodeValidation},
		{"html", []byte("<html><script>alert(1)</script>"), "", apperr.CodeValidation},
		{"%PDF без дефиса", []byte("%PDF1.4"), "", apperr.CodeValidation},
		{"пустой", nil, "", apperr.CodeValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst bytes.Buffer
			mime, size, sum, err := receiveFile(&dst, bytes.NewReader(tt.content), limit)
			if tt.wantCode != "" {
				e, ok := apperr.As(err)
				if !ok || e.Code != tt.wantCode {
					t.Fatalf("err = %v, want %s", err, tt.wantCode)
				}
				if tt.wantCode == apperr.CodeValidation && (len(e.Details) == 0 || e.Details[0].Field != "file") {
					t.Fatalf("details = %+v, want field file", e.Details)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			want := sha256.Sum256(tt.content)
			if mime != tt.wantMime || size != int64(len(tt.content)) || !bytes.Equal(sum, want[:]) {
				t.Fatalf("got %s, %d байт, sha %x; want %s, %d, %x", mime, size, sum, tt.wantMime, len(tt.content), want)
			}
			if !bytes.Equal(dst.Bytes(), tt.content) {
				t.Fatal("в dst записано не то содержимое")
			}
		})
	}
}
