package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/pgerr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

// maxDocumentSize — совпадает с CHECK size_bytes в kyc_documents (10 МБ).
const maxDocumentSize = 10 << 20

// sniffLen — сколько байт читается для определения типа (как у http.DetectContentType).
const sniffLen = 512

// documentSignatures — белый список: тип файла определяется по первым байтам, а не по
// расширению или Content-Type части формы. Своя таблица, а не http.DetectContentType:
// сервис не зависит от net/http, а три сигнатуры точнее WHATWG-сниффера.
var documentSignatures = []struct {
	prefix []byte
	mime   string
}{
	{[]byte("%PDF-"), "application/pdf"},
	{[]byte("\x89PNG\r\n\x1a\n"), "image/png"},
	{[]byte{0xFF, 0xD8, 0xFF}, "image/jpeg"},
}

// receiveFile копирует файл из src в dst потоком: в памяти — только первые 512 байт для
// сигнатуры и буфер io.Copy. Возвращает MIME по сигнатуре, размер и sha256.
// Ошибка — 400 (пустой файл, тип не из белого списка) или 413 (больше max). При ошибке
// в dst могла попасть часть файла — его удаляет вызывающий.
func receiveFile(dst io.Writer, src io.Reader, max int64) (mime string, size int64, sum []byte, err error) {
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", 0, nil, readError(err)
	}
	head = head[:n]
	if n == 0 {
		return "", 0, nil, fileValidation("пустой файл")
	}
	mime = sniffDocument(head)
	if mime == "" {
		return "", 0, nil, fileValidation("допустимы только PDF, PNG, JPEG")
	}

	hash := sha256.New()
	// max+1: прочитать на байт больше лимита — единственный способ отличить «ровно лимит»
	// от «больше», не доверяя Content-Length.
	limited := io.LimitReader(io.MultiReader(bytes.NewReader(head), src), max+1)
	size, err = io.Copy(dst, io.TeeReader(limited, hash))
	if err != nil {
		return "", 0, nil, readError(err)
	}
	if size > max {
		return "", 0, nil, apperr.FileTooLarge()
	}
	return mime, size, hash.Sum(nil), nil
}

func sniffDocument(head []byte) string {
	for _, sig := range documentSignatures {
		if bytes.HasPrefix(head, sig.prefix) {
			return sig.mime
		}
	}
	return ""
}

func fileValidation(msg string) *apperr.Error {
	return apperr.Validation("Файл не принят", apperr.FieldError{Field: "file", Message: msg})
}

// readError — ошибка чтения тела. apperr (например, FileTooLarge от лимита тела запроса,
// который переводит хендлер) отдаётся как есть, остальное — 500 с контекстом.
func readError(err error) error {
	if _, ok := apperr.As(err); ok {
		return err
	}
	return fmt.Errorf("receive file: %w", err)
}

// maxFileNameRunes — совпадает с CHECK length(file_name) <= 255.
const maxFileNameRunes = 255

// defaultFileName — если от имени ничего не осталось.
const defaultFileName = "document"

// UploadDocument принимает KYC-документ: файл пишется потоком в хранилище, строка — в
// БД. Порядок «сначала файл, потом строка»: осиротевший файл безвреден, а строка без
// файла — 404 на скачивании. Любая ошибка после создания файла его удаляет.
func (k *Kyc) UploadDocument(ctx context.Context, p Principal, meta ClientMeta, docType repository.KycDocumentType, fileName string, content io.Reader) (repository.KycDocument, error) {
	orgID, err := ownOrganization(p)
	if err != nil {
		return repository.KycDocument{}, err
	}
	// Ранний 409 — до приёма 10 МБ. Окончательная проверка — под блокировкой ниже.
	org, err := k.store.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return repository.KycDocument{}, pgerr.Map(err, "Организация не найдена")
	}
	if _, err := nextKycStatus(org.KycStatus, KycEdit); err != nil {
		return repository.KycDocument{}, err
	}

	key, w, err := k.files.Create()
	if err != nil {
		return repository.KycDocument{}, fmt.Errorf("create document file: %w", err)
	}
	doc, err := k.storeDocument(ctx, p, meta, orgID, docType, sanitizeFileName(fileName), key, w, content)
	if err != nil {
		if rmErr := k.files.Remove(key); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			slog.WarnContext(ctx, "remove orphan document file", "storage_key", key, "err", rmErr)
		}
		return repository.KycDocument{}, err
	}
	return doc, nil
}

// storeDocument дописывает файл и создаёт строку. Статус проверяется повторно под
// блокировкой: submit мог пройти, пока принимался файл.
func (k *Kyc) storeDocument(ctx context.Context, p Principal, meta ClientMeta, orgID uuid.UUID,
	docType repository.KycDocumentType, fileName, key string, w io.WriteCloser, content io.Reader,
) (repository.KycDocument, error) {
	mime, size, sum, err := receiveFile(w, content, maxDocumentSize)
	if closeErr := w.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("close document file: %w", closeErr)
	}
	if err != nil {
		return repository.KycDocument{}, err
	}

	var doc repository.KycDocument
	err = k.tx.InTx(ctx, func(q repository.Querier) error {
		org, err := q.GetOrganizationForUpdate(ctx, orgID)
		if err != nil {
			return pgerr.Map(err, "Организация не найдена")
		}
		if _, err := nextKycStatus(org.KycStatus, KycEdit); err != nil {
			return err
		}
		doc, err = q.CreateKycDocument(ctx, repository.CreateKycDocumentParams{
			OrganizationID:   orgID,
			DocType:          docType,
			StorageKey:       key,
			FileName:         fileName,
			MimeType:         mime,
			SizeBytes:        size,
			Sha256:           sum,
			UploadedByUserID: p.UserID,
		})
		if err != nil {
			return fmt.Errorf("create kyc document: %w", err)
		}
		return writeAudit(ctx, q, meta, AuditEvent{
			Action:     AuditDocumentUpload,
			UserID:     &p.UserID,
			EntityType: repository.EntityTypeDocument,
			EntityID:   doc.ID.String(),
			Meta:       map[string]any{"doc_type": string(docType), "organization_id": orgID.String()},
		})
	})
	return doc, err
}

// sanitizeFileName — имя только для показа: без управляющих символов (переводы строк
// ломали бы логи и заголовки), не длиннее 255 рун, пустое — "document". На диск имя не
// попадает: файл лежит под случайным ключом.
func sanitizeFileName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	if runes := []rune(clean); len(runes) > maxFileNameRunes {
		clean = string(runes[:maxFileNameRunes])
	}
	if strings.TrimSpace(clean) == "" {
		return defaultFileName
	}
	return clean
}

// DeleteDocument удаляет свой документ (только draft / rejected). Порядок «сначала строка,
// потом файл»: после коммита строки файла уже не видно никому, и сбой удаления файла
// оставляет лишь безвредного сироту — его только логируем.
func (k *Kyc) DeleteDocument(ctx context.Context, p Principal, id uuid.UUID) error {
	orgID, err := ownOrganization(p)
	if err != nil {
		return err
	}
	var key string
	err = k.tx.InTx(ctx, func(q repository.Querier) error {
		org, err := q.GetOrganizationForUpdate(ctx, orgID)
		if err != nil {
			return pgerr.Map(err, "Организация не найдена")
		}
		if _, err := nextKycStatus(org.KycStatus, KycEdit); err != nil {
			return err
		}
		// organization_id в WHERE: чужой документ неотличим от несуществующего → 404.
		key, err = q.DeleteKycDocument(ctx, repository.DeleteKycDocumentParams{ID: id, OrganizationID: orgID})
		return pgerr.Map(err, "Документ не найден")
	})
	if err != nil {
		return err
	}
	if err := k.files.Remove(key); err != nil {
		slog.WarnContext(ctx, "remove deleted document file", "document_id", id, "err", err)
	}
	return nil
}

// DocumentFile — открытый файл документа для отдачи; Body закрывает вызывающий.
type DocumentFile struct {
	Body     io.ReadCloser
	Size     int64
	FileName string
	MimeType string
}

// OpenDocument открывает файл документа: своей организации или любой — для admin.
// Чужой и несуществующий неотличимы (404, IDOR). Скачивание админом пишется в audit_log
// до отдачи файла: транзакции нет, и при сбое записи файл не отдаётся (BE-14: действие
// без следа в журнале хуже отказа). Скачивание владельцем не пишется (DECISIONS п. 16).
func (k *Kyc) OpenDocument(ctx context.Context, p Principal, meta ClientMeta, id uuid.UUID) (DocumentFile, error) {
	doc, err := k.store.GetKycDocumentVisible(ctx, repository.GetKycDocumentVisibleParams{
		ID:         id,
		ViewerRole: p.ViewerRole(),
		ViewerOrg:  p.ViewerOrg(),
	})
	if err != nil {
		return DocumentFile{}, pgerr.Map(err, "Документ не найден")
	}
	body, size, err := k.files.Open(doc.StorageKey)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Строка без файла: например, документы демо-сида до make seed-demo (BE-16).
			slog.WarnContext(ctx, "document file missing", "document_id", id)
			return DocumentFile{}, apperr.NotFound("Файл документа не найден")
		}
		return DocumentFile{}, fmt.Errorf("open document file: %w", err)
	}
	if p.ViewerRole() == repository.UserRoleAdmin {
		err := writeAudit(ctx, k.store, meta, AuditEvent{
			Action:     AuditDocumentDownload,
			UserID:     &p.UserID,
			EntityType: repository.EntityTypeDocument,
			EntityID:   doc.ID.String(),
			Meta:       map[string]any{"organization_id": doc.OrganizationID.String()},
		})
		if err != nil {
			_ = body.Close()
			return DocumentFile{}, err
		}
	}
	return DocumentFile{Body: body, Size: size, FileName: doc.FileName, MimeType: doc.MimeType}, nil
}

// ListDocuments — документы своей организации по порядку загрузки.
func (k *Kyc) ListDocuments(ctx context.Context, p Principal) ([]repository.KycDocument, error) {
	orgID, err := ownOrganization(p)
	if err != nil {
		return nil, err
	}
	docs, err := k.store.ListKycDocumentsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list kyc documents: %w", err)
	}
	return docs, nil
}
