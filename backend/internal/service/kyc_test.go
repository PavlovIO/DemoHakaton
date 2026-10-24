package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/aml"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeKycStore — БД одной организации в памяти. Транзакция (fakeKycTx) работает с ней же:
// откат не моделируется, тесты проверяют, что дошло до вызовов, а не до коммита.
type fakeKycStore struct {
	repository.Querier
	org           repository.Organization
	beneficiaries []repository.Beneficiary
	documents     []repository.KycDocument
	audit         []repository.CreateAuditLogParams
	locked        bool   // GetOrganizationForUpdate вызван
	onLock        func() // вызывается до чтения под блокировкой: «параллельный» запрос
	createDocErr  error
	committed     bool // fakeKycTx: колбэк транзакции вернул nil
}

func newFakeKycStore(status repository.KycStatus) *fakeKycStore {
	return &fakeKycStore{org: repository.Organization{
		ID:                 uuid.New(),
		Type:               repository.OrganizationTypeBorrower,
		Name:               "ООО Альфа",
		CountryCode:        "RU",
		RegistrationNumber: "7707083893",
		LegalAddress:       "Москва",
		KycStatus:          status,
	}}
}

func (s *fakeKycStore) principal() service.Principal {
	orgID := s.org.ID
	return service.Principal{UserID: uuid.New(), OrgID: &orgID, Role: string(s.org.Type), KycStatus: string(s.org.KycStatus)}
}

func (s *fakeKycStore) GetOrganizationByID(_ context.Context, id uuid.UUID) (repository.Organization, error) {
	if id != s.org.ID {
		return repository.Organization{}, pgx.ErrNoRows
	}
	return s.org, nil
}

func (s *fakeKycStore) GetOrganizationForUpdate(ctx context.Context, id uuid.UUID) (repository.Organization, error) {
	s.locked = true
	if s.onLock != nil {
		s.onLock()
	}
	return s.GetOrganizationByID(ctx, id)
}

func (s *fakeKycStore) UpdateOrganization(_ context.Context, arg repository.UpdateOrganizationParams) (repository.Organization, error) {
	if arg.ID != s.org.ID || (s.org.KycStatus != repository.KycStatusDraft && s.org.KycStatus != repository.KycStatusRejected) {
		return repository.Organization{}, pgx.ErrNoRows
	}
	if arg.Name.Valid {
		s.org.Name = arg.Name.String
	}
	if arg.RegistrationNumber.Valid {
		s.org.RegistrationNumber = arg.RegistrationNumber.String
	}
	if arg.LegalAddress.Valid {
		s.org.LegalAddress = arg.LegalAddress.String
	}
	if arg.Lei.Valid {
		s.org.Lei = pgtype.Text{String: arg.Lei.String, Valid: arg.Lei.String != ""}
	}
	return s.org, nil
}

func (s *fakeKycStore) ListBeneficiariesByOrg(_ context.Context, orgID uuid.UUID) ([]repository.Beneficiary, error) {
	var out []repository.Beneficiary
	for _, b := range s.beneficiaries {
		if b.OrganizationID == orgID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (s *fakeKycStore) DeleteBeneficiariesByOrg(_ context.Context, orgID uuid.UUID) error {
	kept := s.beneficiaries[:0]
	for _, b := range s.beneficiaries {
		if b.OrganizationID != orgID {
			kept = append(kept, b)
		}
	}
	s.beneficiaries = kept
	return nil
}

func (s *fakeKycStore) CreateBeneficiary(_ context.Context, arg repository.CreateBeneficiaryParams) (repository.Beneficiary, error) {
	b := repository.Beneficiary{
		ID: uuid.New(), OrganizationID: arg.OrganizationID, FullName: arg.FullName,
		CitizenshipCountryCode: arg.CitizenshipCountryCode, OwnershipPercent: arg.OwnershipPercent,
	}
	s.beneficiaries = append(s.beneficiaries, b)
	return b, nil
}

func (s *fakeKycStore) CreateKycDocument(_ context.Context, arg repository.CreateKycDocumentParams) (repository.KycDocument, error) {
	if s.createDocErr != nil {
		return repository.KycDocument{}, s.createDocErr
	}
	d := repository.KycDocument{
		ID: uuid.New(), OrganizationID: arg.OrganizationID, DocType: arg.DocType, StorageKey: arg.StorageKey,
		FileName: arg.FileName, MimeType: arg.MimeType, SizeBytes: arg.SizeBytes, Sha256: arg.Sha256,
		UploadedByUserID: arg.UploadedByUserID,
	}
	s.documents = append(s.documents, d)
	return d, nil
}

func (s *fakeKycStore) DeleteKycDocument(_ context.Context, arg repository.DeleteKycDocumentParams) (string, error) {
	for i, d := range s.documents {
		if d.ID == arg.ID && d.OrganizationID == arg.OrganizationID {
			s.documents = append(s.documents[:i], s.documents[i+1:]...)
			return d.StorageKey, nil
		}
	}
	return "", pgx.ErrNoRows
}

func (s *fakeKycStore) GetKycDocumentVisible(_ context.Context, arg repository.GetKycDocumentVisibleParams) (repository.KycDocument, error) {
	for _, d := range s.documents {
		if d.ID == arg.ID && (arg.ViewerRole == repository.UserRoleAdmin || d.OrganizationID == arg.ViewerOrg) {
			return d, nil
		}
	}
	return repository.KycDocument{}, pgx.ErrNoRows
}

func (s *fakeKycStore) CreateAuditLog(_ context.Context, arg repository.CreateAuditLogParams) (repository.AuditLog, error) {
	s.audit = append(s.audit, arg)
	return repository.AuditLog{}, nil
}

// fakeFiles — хранилище файлов в памяти; помнит удалённые ключи.
type fakeFiles struct {
	files    map[string][]byte
	removed  []string
	next     int
	onRemove func(key string)
}

func newFakeFiles() *fakeFiles { return &fakeFiles{files: map[string][]byte{}} }

type fakeFile struct {
	bytes.Buffer
	commit func([]byte)
}

func (f *fakeFile) Close() error { f.commit(f.Bytes()); return nil }

func (f *fakeFiles) Create() (string, io.WriteCloser, error) {
	f.next++
	key := fmt.Sprintf("key-%d", f.next)
	return key, &fakeFile{commit: func(b []byte) { f.files[key] = b }}, nil
}

func (f *fakeFiles) Open(key string) (io.ReadCloser, int64, error) {
	b, ok := f.files[key]
	if !ok {
		return nil, 0, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), int64(len(b)), nil
}

func (f *fakeFiles) Remove(key string) error {
	if f.onRemove != nil {
		f.onRemove(key)
	}
	f.removed = append(f.removed, key)
	if _, ok := f.files[key]; !ok {
		return fs.ErrNotExist
	}
	delete(f.files, key)
	return nil
}

type fakeKycTx struct{ store *fakeKycStore }

func (t fakeKycTx) InTx(_ context.Context, fn func(q repository.Querier) error) error {
	if err := fn(t.store); err != nil {
		return err
	}
	t.store.committed = true
	return nil
}

func newKyc(store *fakeKycStore) *service.Kyc {
	return newKycWithFiles(store, newFakeFiles())
}

func newKycWithFiles(store *fakeKycStore, files *fakeFiles) *service.Kyc {
	return service.NewKyc(service.KycDeps{Store: store, Tx: fakeKycTx{store: store}, Files: files, Screener: aml.NoopScreener{}})
}

func ptr[T any](v T) *T { return &v }

func beneficiary(name, percent string) service.BeneficiaryInput {
	return service.BeneficiaryInput{FullName: name, CitizenshipCountryCode: "RU", OwnershipPercent: decimal.RequireFromString(percent)}
}

func wantCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	if e, ok := apperr.As(err); !ok || e.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

// wantValidationField — 400 VALIDATION_ERROR с записью field в details.
func wantValidationField(t *testing.T, err error, field string) {
	t.Helper()
	wantCode(t, err, apperr.CodeValidation)
	e, _ := apperr.As(err)
	for _, d := range e.Details {
		if d.Field == field {
			return
		}
	}
	t.Fatalf("details = %+v, want field %q", e.Details, field)
}

func TestKycUpdateMyOrganization(t *testing.T) {
	ctx := context.Background()

	t.Run("draft — новые поля и бенефициары", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		got, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{
			Name:          ptr("ООО Альфа Плюс"),
			Beneficiaries: &[]service.BeneficiaryInput{beneficiary("Иванов И.И.", "60"), beneficiary("Петров П.П.", "40")},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Org.Name != "ООО Альфа Плюс" || len(got.Beneficiaries) != 2 || got.Beneficiaries[0].FullName != "Иванов И.И." {
			t.Fatalf("profile = %+v", got)
		}
		if !store.locked {
			t.Error("строка организации не заблокирована (GetOrganizationForUpdate)")
		}
	})

	t.Run("under_review — 409", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusUnderReview)
		_, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{Name: ptr("Другое имя")})
		wantCode(t, err, apperr.CodeInvalidTransition)
		if store.org.Name != "ООО Альфа" {
			t.Errorf("имя изменилось: %q", store.org.Name)
		}
	})

	t.Run("сумма долей 60 + 50 — 400", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		_, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{
			Beneficiaries: &[]service.BeneficiaryInput{beneficiary("А", "60"), beneficiary("Б", "50")},
		})
		wantValidationField(t, err, "beneficiaries")
	})

	t.Run("beneficiaries не передан — список не тронут", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusRejected)
		store.beneficiaries = []repository.Beneficiary{{ID: uuid.New(), OrganizationID: store.org.ID, FullName: "Иванов И.И."}}
		got, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{LegalAddress: ptr("Санкт-Петербург")})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got.Beneficiaries) != 1 || len(store.beneficiaries) != 1 {
			t.Fatalf("бенефициары = %+v, want один, не тронутый", got.Beneficiaries)
		}
	})

	t.Run("beneficiaries: [] — список очищен", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		store.beneficiaries = []repository.Beneficiary{{ID: uuid.New(), OrganizationID: store.org.ID, FullName: "Иванов И.И."}}
		got, err := newKyc(store).UpdateMyOrganization(ctx, store.principal(), service.OrganizationUpdateInput{Beneficiaries: &[]service.BeneficiaryInput{}})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got.Beneficiaries) != 0 || len(store.beneficiaries) != 0 {
			t.Fatalf("бенефициары = %+v, want пусто", got.Beneficiaries)
		}
	})
}

func TestKycMyOrganization(t *testing.T) {
	ctx := context.Background()

	t.Run("без бенефициаров — пустой срез", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		got, err := newKyc(store).MyOrganization(ctx, store.principal())
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.Org.ID != store.org.ID || got.Beneficiaries == nil {
			t.Fatalf("profile = %+v, want своя организация и не-nil бенефициары", got)
		}
	})

	t.Run("admin без организации — 403", func(t *testing.T) {
		store := newFakeKycStore(repository.KycStatusDraft)
		_, err := newKyc(store).MyOrganization(ctx, service.Principal{UserID: uuid.New(), Role: "admin"})
		wantCode(t, err, apperr.CodeForbidden)
	})
}

const testPDF = "%PDF-1.4\nтестовый документ"

func TestUploadDocument(t *testing.T) {
	ctx := context.Background()
	meta := service.ClientMeta{UserAgent: "test-agent"}

	t.Run("успех — файл, строка и аудит", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusDraft), newFakeFiles()
		doc, err := newKycWithFiles(store, files).UploadDocument(ctx, store.principal(), meta,
			repository.KycDocumentTypeCharter, "устав.pdf", strings.NewReader(testPDF))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if doc.MimeType != "application/pdf" || doc.SizeBytes != int64(len(testPDF)) || len(doc.Sha256) != 32 || doc.FileName != "устав.pdf" {
			t.Fatalf("doc = %+v", doc)
		}
		if string(files.files[doc.StorageKey]) != testPDF {
			t.Fatalf("файл под ключом %q не сохранён", doc.StorageKey)
		}
		if len(store.audit) != 1 || store.audit[0].Action != "document.upload" || store.audit[0].EntityID.String != doc.ID.String() {
			t.Fatalf("audit = %+v", store.audit)
		}
		var auditMeta map[string]string
		_ = json.Unmarshal(store.audit[0].Meta, &auditMeta)
		if auditMeta["doc_type"] != "charter" || auditMeta["organization_id"] != store.org.ID.String() {
			t.Fatalf("audit meta = %s", store.audit[0].Meta)
		}
	})

	t.Run("under_review — 409 до приёма файла", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusUnderReview), newFakeFiles()
		_, err := newKycWithFiles(store, files).UploadDocument(ctx, store.principal(), meta,
			repository.KycDocumentTypeCharter, "a.pdf", strings.NewReader(testPDF))
		wantCode(t, err, apperr.CodeInvalidTransition)
		if files.next != 0 {
			t.Fatal("файл создан, хотя статус не позволяет загрузку")
		}
	})

	t.Run("ошибка CreateKycDocument — файл удалён", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusDraft), newFakeFiles()
		store.createDocErr = errors.New("db down")
		_, err := newKycWithFiles(store, files).UploadDocument(ctx, store.principal(), meta,
			repository.KycDocumentTypeCharter, "a.pdf", strings.NewReader(testPDF))
		if err == nil {
			t.Fatal("want error")
		}
		if len(files.removed) != 1 || files.removed[0] != "key-1" || len(files.files) != 0 {
			t.Fatalf("removed = %v, files = %v", files.removed, files.files)
		}
	})

	t.Run("submit прошёл, пока шёл файл — 409 и файл удалён", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusDraft), newFakeFiles()
		store.onLock = func() { store.org.KycStatus = repository.KycStatusUnderReview }
		_, err := newKycWithFiles(store, files).UploadDocument(ctx, store.principal(), meta,
			repository.KycDocumentTypeCharter, "a.pdf", strings.NewReader(testPDF))
		wantCode(t, err, apperr.CodeInvalidTransition)
		if len(files.removed) != 1 || len(store.documents) != 0 {
			t.Fatalf("removed = %v, documents = %d", files.removed, len(store.documents))
		}
	})

	t.Run("файл не PDF/PNG/JPEG — 400 и файл удалён", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusDraft), newFakeFiles()
		_, err := newKycWithFiles(store, files).UploadDocument(ctx, store.principal(), meta,
			repository.KycDocumentTypeCharter, "a.pdf", strings.NewReader("<html>"))
		wantValidationField(t, err, "file")
		if len(files.removed) != 1 {
			t.Fatalf("removed = %v", files.removed)
		}
	})
}

func TestUploadDocumentFileName(t *testing.T) {
	long := strings.Repeat("я", 300)
	tests := []struct{ in, want string }{
		{"устав.pdf", "устав.pdf"},
		{"a\x00b\r\nc\t.pdf", "abc.pdf"},
		{"", "document"},
		{"\x01\x02", "document"},
		{long, strings.Repeat("я", 255)},
	}
	for _, tt := range tests {
		store := newFakeKycStore(repository.KycStatusDraft)
		doc, err := newKyc(store).UploadDocument(context.Background(), store.principal(), service.ClientMeta{},
			repository.KycDocumentTypeOther, tt.in, strings.NewReader(testPDF))
		if err != nil {
			t.Fatalf("%q: err = %v", tt.in, err)
		}
		if doc.FileName != tt.want {
			t.Errorf("FileName(%q) = %q, want %q", tt.in, doc.FileName, tt.want)
		}
	}
}

// withDocument — документ своей организации: строка в сторе и файл в хранилище.
func withDocument(store *fakeKycStore, files *fakeFiles) repository.KycDocument {
	key, w, _ := files.Create()
	_, _ = w.Write([]byte(testPDF))
	_ = w.Close()
	d := repository.KycDocument{
		ID: uuid.New(), OrganizationID: store.org.ID, DocType: repository.KycDocumentTypeCharter,
		StorageKey: key, FileName: "устав.pdf", MimeType: "application/pdf", SizeBytes: int64(len(testPDF)),
	}
	store.documents = append(store.documents, d)
	return d
}

func TestDeleteDocument(t *testing.T) {
	ctx := context.Background()

	t.Run("чужой id — 404", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusDraft), newFakeFiles()
		other := withDocument(store, files)
		other.OrganizationID = uuid.New()
		store.documents[0] = other
		err := newKycWithFiles(store, files).DeleteDocument(ctx, store.principal(), other.ID)
		wantCode(t, err, apperr.CodeNotFound)
		if len(files.removed) != 0 {
			t.Fatalf("удалён чужой файл: %v", files.removed)
		}
	})

	t.Run("under_review — 409, строка и файл на месте", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusUnderReview), newFakeFiles()
		doc := withDocument(store, files)
		err := newKycWithFiles(store, files).DeleteDocument(ctx, store.principal(), doc.ID)
		wantCode(t, err, apperr.CodeInvalidTransition)
		if len(store.documents) != 1 || len(files.files) != 1 {
			t.Fatalf("documents = %d, files = %d", len(store.documents), len(files.files))
		}
	})

	t.Run("успех — файл удалён после коммита", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusRejected), newFakeFiles()
		doc := withDocument(store, files)
		files.onRemove = func(string) {
			if !store.committed {
				t.Error("файл удалён до коммита")
			}
		}
		if err := newKycWithFiles(store, files).DeleteDocument(ctx, store.principal(), doc.ID); err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(store.documents) != 0 || len(files.removed) != 1 || files.removed[0] != doc.StorageKey {
			t.Fatalf("documents = %d, removed = %v", len(store.documents), files.removed)
		}
	})
}

func TestDownloadDocument(t *testing.T) {
	ctx := context.Background()
	meta := service.ClientMeta{UserAgent: "test-agent"}
	read := func(t *testing.T, f service.DocumentFile) string {
		t.Helper()
		defer f.Body.Close()
		b, _ := io.ReadAll(f.Body)
		return string(b)
	}

	t.Run("свой — файл, аудита нет", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusVerified), newFakeFiles()
		doc := withDocument(store, files)
		f, err := newKycWithFiles(store, files).OpenDocument(ctx, store.principal(), meta, doc.ID)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if read(t, f) != testPDF || f.Size != int64(len(testPDF)) || f.FileName != "устав.pdf" || f.MimeType != "application/pdf" {
			t.Fatalf("file = %+v", f)
		}
		if len(store.audit) != 0 {
			t.Fatalf("аудит скачивания владельцем: %+v", store.audit)
		}
	})

	t.Run("чужой — 404", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusVerified), newFakeFiles()
		doc := withDocument(store, files)
		otherOrg := uuid.New()
		stranger := service.Principal{UserID: uuid.New(), OrgID: &otherOrg, Role: "borrower"}
		_, err := newKycWithFiles(store, files).OpenDocument(ctx, stranger, meta, doc.ID)
		wantCode(t, err, apperr.CodeNotFound)
	})

	t.Run("admin — файл и одна запись document.download", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusUnderReview), newFakeFiles()
		doc := withDocument(store, files)
		admin := service.Principal{UserID: uuid.New(), Role: "admin"}
		f, err := newKycWithFiles(store, files).OpenDocument(ctx, admin, meta, doc.ID)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		_ = read(t, f)
		if len(store.audit) != 1 || store.audit[0].Action != "document.download" || store.audit[0].EntityID.String != doc.ID.String() {
			t.Fatalf("audit = %+v", store.audit)
		}
		var auditMeta map[string]string
		_ = json.Unmarshal(store.audit[0].Meta, &auditMeta)
		if auditMeta["organization_id"] != store.org.ID.String() {
			t.Fatalf("audit meta = %s", store.audit[0].Meta)
		}
	})

	t.Run("файла нет на диске — 404", func(t *testing.T) {
		store, files := newFakeKycStore(repository.KycStatusVerified), newFakeFiles()
		doc := withDocument(store, files)
		delete(files.files, doc.StorageKey)
		_, err := newKycWithFiles(store, files).OpenDocument(ctx, store.principal(), meta, doc.ID)
		wantCode(t, err, apperr.CodeNotFound)
	})
}
