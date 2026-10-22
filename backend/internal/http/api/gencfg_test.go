package api_test

import (
	"os"
	"strings"
	"testing"
)

// server.gen.go каждого <tag>api ссылается на типы models.gen.go (dot-import пакета api),
// поэтому настройки, влияющие на имена и типы (compatibility, output-options), обязаны
// совпадать дословно в обоих конфигах: иначе он сошлётся на типы, которых в models.gen.go нет. extends у oapi-codegen нет — блок дублируется
// между маркерами, а этот тест следит, чтобы копии не разъехались.
func TestGeneratorConfigsShareCommonBlock(t *testing.T) {
	const (
		begin = "# ───── общее начало"
		end   = "# ───── общее конец ─────"
	)
	configs := []string{"../../../oapi-codegen/models.yaml", "../../../oapi-codegen/server.yaml"}

	blocks := make(map[string]string, len(configs))
	for _, path := range configs {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, after, ok := strings.Cut(string(data), begin)
		if !ok {
			t.Fatalf("%s: нет маркера %q", path, begin)
		}
		block, _, ok := strings.Cut(after, end)
		if !ok {
			t.Fatalf("%s: нет маркера %q", path, end)
		}
		blocks[path] = strings.ReplaceAll(block, "\r\n", "\n")
	}

	ref := blocks[configs[0]]
	for _, path := range configs[1:] {
		if blocks[path] != ref {
			t.Errorf("общий блок в %s отличается от %s:\n--- %s\n%s\n--- %s\n%s",
				path, configs[0], configs[0], ref, path, blocks[path])
		}
	}
}
