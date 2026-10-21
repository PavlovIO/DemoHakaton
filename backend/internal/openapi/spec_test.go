package openapi_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/openapi"
)

func TestEmbeddedSpecIsValid(t *testing.T) {
	doc, err := openapi.Doc()
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("встроенная спека не проходит валидацию: %v", err)
	}
	if doc.Paths.Find("/healthz") == nil {
		t.Fatal("в спеке нет /healthz")
	}
	if len(doc.Servers) != 0 {
		t.Fatalf("Servers должны быть обнулены (префикс снимает middleware), есть %d", len(doc.Servers))
	}
	if doc.Components.SecuritySchemes["cookieAuth"] == nil {
		t.Fatal("в спеке нет securitySchemes.cookieAuth")
	}
}

func TestEmbeddedSpecEqualsRepoBundle(t *testing.T) {
	// Копия для go:embed пишется specbundle одновременно с api/openapi.yaml;
	// расхождение — забытый make gen. CI ловит его через git diff, этот тест — локально.
	want, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(openapi.Spec, want) {
		t.Fatal("backend/internal/openapi/openapi.yaml отличается от api/openapi.yaml: запустите make gen")
	}
}
