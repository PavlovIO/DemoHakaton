package main

import (
	"bytes"
	"strings"
	"testing"
)

// Реальный корень собирается, а повторная сборка даёт те же байты (идемпотентность).
func TestBundleRealSpecIsDeterministic(t *testing.T) {
	a, err := bundle("../../../api/openapi.root.yaml")
	if err != nil {
		t.Fatal(err)
	}
	b, err := bundle("../../../api/openapi.root.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("две сборки подряд дали разные байты")
	}
	if !bytes.HasPrefix(a, []byte(banner)) {
		t.Fatal("нет баннера")
	}
}

// Резолвер называет компоненты последним сегментом фрагмента, поэтому одноимённые
// компоненты из разных мест обязаны быть ошибкой, а не молчаливым схлопыванием.
func TestBundleRejectsNameCollisions(t *testing.T) {
	tests := []struct{ name, root, want string }{
		{"разные фрагменты одного файла", "testdata/collision_fragment/root.yaml", "schemas/status"},
		{"компонент корня и внешний", "testdata/collision_root/root.yaml", "schemas/Error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := bundle(tt.root)
			if err == nil {
				t.Fatal("ожидалась ошибка коллизии имён")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ошибка %q не упоминает %q", err, tt.want)
			}
		})
	}
}
