package storage_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/storage"
)

func TestLocalStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "documents")
	store, err := storage.NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	t.Run("create → open → remove", func(t *testing.T) {
		key, w, err := store.Create()
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(key) != 32 {
			t.Errorf("key = %q, want 32 hex-символа", key)
		}
		content := []byte("%PDF-1.4 test")
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" { // права POSIX на Windows не отображаются
			info, err := os.Stat(filepath.Join(dir, key))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("права файла %v, want 0600", info.Mode().Perm())
			}
		}

		r, size, err := store.Open(key)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		got, _ := io.ReadAll(r)
		_ = r.Close()
		if string(got) != string(content) || size != int64(len(content)) {
			t.Errorf("Open = %q (%d байт), want %q", got, size, content)
		}

		if err := store.Remove(key); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if _, _, err := store.Open(key); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open после Remove: err = %v, want fs.ErrNotExist", err)
		}
	})

	t.Run("ключи уникальны", func(t *testing.T) {
		k1, w1, _ := store.Create()
		k2, w2, _ := store.Create()
		_, _ = w1.Close(), w2.Close()
		if k1 == k2 {
			t.Fatalf("одинаковые ключи %q", k1)
		}
	})

	t.Run("traversal", func(t *testing.T) {
		for _, key := range []string{"../x", "a/b", `a\b`, "", ".", ".."} {
			if _, _, err := store.Open(key); err == nil || errors.Is(err, fs.ErrNotExist) {
				t.Errorf("Open(%q): err = %v, want ошибка недопустимого ключа", key, err)
			}
			if err := store.Remove(key); err == nil {
				t.Errorf("Remove(%q) без ошибки", key)
			}
		}
	})
}
