// Package storage хранит файлы KYC-документов. Сервис зависит от своего интерфейса
// (service.DocumentFiles), поэтому локальный каталог можно заменить на S3, не трогая сервис.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// keyBytes — случайных байт в ключе: 128 бит, имя файла не угадать перебором.
const keyBytes = 16

// errInvalidKey — ключ мог бы выйти за пределы каталога (подменённый storage_key в БД).
var errInvalidKey = errors.New("storage: недопустимый ключ")

// Local — файлы в локальном каталоге под случайными именами. Каталог не раздаётся
// статикой: файл отдаётся только через downloadDocument с проверкой доступа.
type Local struct {
	dir string
}

// NewLocal создаёт каталог (0700) и проверяет, что в него можно писать. Ошибка —
// сервер не стартует: без хранилища загрузка документов сломалась бы на первом запросе.
func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("storage: каталог %s: %w", dir, err)
	}
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return nil, fmt.Errorf("storage: каталог %s недоступен для записи: %w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	if err := os.Remove(name); err != nil {
		return nil, fmt.Errorf("storage: удалить пробный файл: %w", err)
	}
	return &Local{dir: dir}, nil
}

// Create открывает новый файл (0600) под случайным ключом. Ключ не связан с именем
// файла пользователя: расширение и путь из формы на диск не попадают.
func (l *Local) Create() (string, io.WriteCloser, error) {
	buf := make([]byte, keyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("storage: ключ: %w", err)
	}
	key := hex.EncodeToString(buf)
	// O_EXCL: при совпадении ключа (практически невозможно) — ошибка, а не перезапись.
	f, err := os.OpenFile(filepath.Join(l.dir, key), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("storage: создать файл: %w", err)
	}
	return key, f, nil
}

// Open открывает файл на чтение и возвращает его размер. Нет файла — fs.ErrNotExist.
func (l *Local) Open(key string) (io.ReadCloser, int64, error) {
	path, err := l.path(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, fmt.Errorf("storage: stat: %w", err)
	}
	return f, info.Size(), nil
}

// Remove удаляет файл. Нет файла — fs.ErrNotExist.
func (l *Local) Remove(key string) error {
	path, err := l.path(key)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// path — путь файла внутри каталога. Ключ из БД не доверенный: «..», разделители и
// пустая строка отвергаются до filepath.Join — защита от path traversal.
func (l *Local) path(key string) (string, error) {
	if key == "" || key == "." || strings.ContainsAny(key, `/\`) || !filepath.IsLocal(key) {
		return "", fmt.Errorf("%w: %q", errInvalidKey, key)
	}
	return filepath.Join(l.dir, key), nil
}
