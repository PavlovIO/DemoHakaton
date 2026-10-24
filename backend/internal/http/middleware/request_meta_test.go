package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWithRequestMeta(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.14:4567"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	req.Header.Set("User-Agent", strings.Repeat("я", 256))

	var gotIP string
	var gotUserAgent string
	handler := WithRequestMeta()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		meta := ClientMetaFromContext(r.Context())
		if meta.IP != nil {
			gotIP = meta.IP.String()
		}
		gotUserAgent = meta.UserAgent
	}))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotIP != "192.0.2.14" {
		t.Fatalf("IP = %q, want remote address IP", gotIP)
	}
	if gotIP == "203.0.113.99" {
		t.Fatal("trusted X-Forwarded-For unexpectedly")
	}
	if count := utf8.RuneCountInString(gotUserAgent); count != 255 {
		t.Fatalf("User-Agent rune count = %d, want 255", count)
	}
}

func TestWithRequestMetaIgnoresInvalidRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "invalid"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")

	var hasIP bool
	handler := WithRequestMeta()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hasIP = ClientMetaFromContext(r.Context()).IP != nil
	}))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if hasIP {
		t.Fatal("invalid RemoteAddr produced an IP")
	}
}
