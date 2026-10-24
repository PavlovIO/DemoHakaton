// Команда seedfiles — dev-утилита сида (BE-16): кладёт демо-PDF в хранилище документов
// BE-05 под storage_key из сида, чтобы документы демо-компаний скачивались.
//
//	go -C backend run ./cmd/seedfiles -manifest migrations/testdata/demo_files/manifest.txt -dst <DOCUMENT_STORAGE_DIR>
//
// Манифест — строки «<storage_key> <файл>», файл ищется рядом с манифестом. Вместо cp и
// mkdir -p в Makefile — Go: make должен работать и в cmd.exe.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/storage"
)

// storageKey — формат ключа BE-05: 32 hex-символа, без расширения и разделителей пути.
var storageKey = regexp.MustCompile(`^[0-9a-f]{32}$`)

func main() {
	manifest := flag.String("manifest", "", "манифест demo_files/manifest.txt")
	dst := flag.String("dst", "", "каталог хранилища документов (DOCUMENT_STORAGE_DIR)")
	flag.Parse()
	if *manifest == "" || *dst == "" {
		flag.Usage()
		os.Exit(2)
	}
	n, err := copyManifest(*manifest, *dst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seedfiles:", err)
		os.Exit(1)
	}
	fmt.Printf("seedfiles: %d files → %s\n", n, *dst)
}

// copyManifest копирует файлы манифеста в dst под их ключами и возвращает их число.
// Существующие файлы перезаписываются: повторный сид не падает.
func copyManifest(manifestPath, dst string) (int, error) {
	// NewLocal создаёт каталог с правами 0700, как сервер BE-05.
	if _, err := storage.NewLocal(dst); err != nil {
		return 0, err
	}
	f, err := os.Open(manifestPath)
	if err != nil {
		return 0, fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()

	srcDir := filepath.Dir(manifestPath)
	copied := 0
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		key, file, ok := strings.Cut(text, " ")
		if !ok || !storageKey.MatchString(key) || !filepath.IsLocal(file) {
			return copied, fmt.Errorf("manifest line %d: want «<32 hex key> <file>», got %q", line, text)
		}
		if err := copyFile(filepath.Join(srcDir, file), filepath.Join(dst, key)); err != nil {
			return copied, fmt.Errorf("manifest line %d: %w", line, err)
		}
		copied++
	}
	if err := sc.Err(); err != nil {
		return copied, fmt.Errorf("read manifest: %w", err)
	}
	return copied, nil
}

// copyFile пишет файл с правами 0600, как BE-05: документы KYC читает только сервер.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %s: %w", src, err)
	}
	return out.Close()
}
