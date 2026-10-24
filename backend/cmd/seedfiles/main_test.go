package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	testdataDir  = "../../migrations/testdata/"
	manifestPath = testdataDir + "demo_files/manifest.txt"
)

func TestCopyManifest(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "storage", "documents")
	n, err := copyManifest(manifestPath, dst)
	if err != nil {
		t.Fatalf("copyManifest() err = %v", err)
	}
	manifest := readManifest(t)
	if n != len(manifest) {
		t.Errorf("copied %d files, manifest has %d", n, len(manifest))
	}
	for key, file := range manifest {
		got, err := os.ReadFile(filepath.Join(dst, key))
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		want, err := os.ReadFile(filepath.Join(testdataDir, "demo_files", file))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s: content differs from %s", key, file)
		}
	}
	// Права как у BE-05: каталог 0700, файлы 0600. На Windows Go их не различает.
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(dst); info.Mode().Perm() != 0o700 {
			t.Errorf("dir mode = %v, want 0700", info.Mode().Perm())
		}
		for key := range manifest {
			if info, _ := os.Stat(filepath.Join(dst, key)); info.Mode().Perm() != 0o600 {
				t.Errorf("%s mode = %v, want 0600", key, info.Mode().Perm())
			}
		}
	}

	// Повторный сид перезаписывает файлы, а не падает.
	if _, err := copyManifest(manifestPath, dst); err != nil {
		t.Errorf("second copyManifest() err = %v", err)
	}
}

func TestCopyManifestRejectsUnsafeKey(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "manifest.txt")
	if err := os.WriteFile(manifest, []byte("../escape demo.pdf\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := copyManifest(manifest, filepath.Join(dir, "dst")); err == nil {
		t.Error("copyManifest() with ../ key: err = nil, want error")
	}
}

// Строки сида: документ ('<storage_key>', '<file_name>', '<demo-файл>') и описание
// demo-файла ('<demo-файл>', <size_bytes>, '<sha256>').
var (
	seedDocumentRow = regexp.MustCompile(`'([0-9a-f]{32})', '[^']+', '(demo_[a-z_]+\.pdf)'\)`)
	seedFileRow     = regexp.MustCompile(`\('(demo_[a-z_]+\.pdf)', (\d+), '([0-9a-f]{64})'\)`)
)

// Документ из сида скачивается (GET /documents/{id}/file → 200), только если файл с его
// storage_key лежит в хранилище и совпадает с size_bytes и sha256 строки: BE-05 сверяет их.
func TestSeedDemoFiles(t *testing.T) {
	manifest := readManifest(t)
	referenced := map[string]bool{}
	for _, seed := range []string{"seed_demo_common.sql", "seed_demo_a.sql", "seed_demo_b.sql"} {
		sql, err := os.ReadFile(testdataDir + seed)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range seedFileRow.FindAllStringSubmatch(string(sql), -1) {
			body, err := os.ReadFile(filepath.Join(testdataDir, "demo_files", row[1]))
			if err != nil {
				t.Fatalf("%s: %v", seed, err)
			}
			sum := sha256.Sum256(body)
			if size, _ := strconv.Atoi(row[2]); size != len(body) || row[3] != hex.EncodeToString(sum[:]) {
				t.Errorf("%s: %s size/sha256 in seed = %s/%s, file = %d/%x", seed, row[1], row[2], row[3], len(body), sum)
			}
		}
		docs := seedDocumentRow.FindAllStringSubmatch(string(sql), -1)
		if len(docs) == 0 {
			t.Errorf("%s: no kyc_documents rows found", seed)
		}
		for _, row := range docs {
			key, file := row[1], row[2]
			referenced[key] = true
			if manifest[key] != file {
				t.Errorf("%s: storage_key %s → %s, manifest → %q", seed, key, file, manifest[key])
			}
		}
	}
	for key := range manifest {
		if !referenced[key] {
			t.Errorf("manifest key %s is not used by any seed", key)
		}
	}
}

func readManifest(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if key, file, ok := strings.Cut(sc.Text(), " "); ok {
			out[key] = file
		}
	}
	return out
}
