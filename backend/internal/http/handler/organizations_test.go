package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
)

// fakeKyc запоминает, с чем его вызвали, и отдаёт заданный профиль или ошибку.
type fakeKyc struct {
	profile   service.OrganizationProfile
	err       error
	gotUpdate *service.OrganizationUpdateInput

	gotDocType  repository.KycDocumentType
	gotFileName string
	gotSize     int64
	file        service.DocumentFile
	deletedID   uuid.UUID
}

func (f *fakeKyc) MyOrganization(context.Context, service.Principal) (service.OrganizationProfile, error) {
	return f.profile, f.err
}

func (f *fakeKyc) UpdateMyOrganization(_ context.Context, _ service.Principal, in service.OrganizationUpdateInput) (service.OrganizationProfile, error) {
	f.gotUpdate = &in
	return f.profile, f.err
}

// UploadDocument читает файл целиком, как настоящий сервис: ошибка чтения (лимит тела)
// возвращается как есть.
func (f *fakeKyc) UploadDocument(_ context.Context, _ service.Principal, _ service.ClientMeta, docType repository.KycDocumentType, fileName string, content io.Reader) (repository.KycDocument, error) {
	f.gotDocType, f.gotFileName = docType, fileName
	n, err := io.Copy(io.Discard, content)
	f.gotSize = n
	if err != nil {
		return repository.KycDocument{}, err
	}
	if f.err != nil {
		return repository.KycDocument{}, f.err
	}
	return repository.KycDocument{
		ID: uuid.New(), DocType: docType, FileName: fileName, MimeType: "application/pdf", SizeBytes: n,
		UploadedAt: pgtype.Timestamptz{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Valid: true},
	}, nil
}

func (f *fakeKyc) ListDocuments(context.Context, service.Principal) ([]repository.KycDocument, error) {
	return nil, f.err
}

func (f *fakeKyc) DeleteDocument(_ context.Context, _ service.Principal, id uuid.UUID) error {
	f.deletedID = id
	return f.err
}

func (f *fakeKyc) SubmitKyc(context.Context, service.Principal, service.ClientMeta) (service.OrganizationProfile, error) {
	return f.profile, f.err
}

func (f *fakeKyc) OpenDocument(context.Context, service.Principal, service.ClientMeta, uuid.UUID) (service.DocumentFile, error) {
	return f.file, f.err
}

func testProfile() service.OrganizationProfile {
	return service.OrganizationProfile{
		Org: &service.Org{
			ID: uuid.New(), Type: "borrower", Name: "ООО Альфа", CountryCode: "RU",
			RegistrationNumber: "7707083893", LegalAddress: "Москва", KycStatus: "draft",
			CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		},
		Beneficiaries: []service.Beneficiary{},
	}
}

func newKycServer(t *testing.T, kyc *fakeKyc) *httptest.Server {
	t.Helper()
	fixture, deps := authFixture()
	deps.Auth = &cookieAuth{fakeAuth: *fixture}
	deps.Dicts, deps.DB, deps.Kyc = fakeDicts{}, okDB{}, kyc
	return newTestServer(t, deps)
}

// borrowerRequest — JSON-запрос заёмщика в draft с сессией и CSRF-токеном.
func borrowerRequest(t *testing.T, srv *httptest.Server, method, path, body string) (*http.Response, []byte) {
	t.Helper()
	ctype := ""
	if body != "" {
		ctype = "application/json"
	}
	return sessionRequest(t, srv, "borrower-draft", method, path, ctype, strings.NewReader(body))
}

// sessionRequest — запрос с cookie session (значение — роль для cookieAuth) и CSRF-токеном.
func sessionRequest(t *testing.T, srv *httptest.Server, session, method, path, ctype string, body io.Reader) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	req.AddCookie(&http.Cookie{Name: middleware.SessionCookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: middleware.CSRFCookieName, Value: "csrf"})
	req.Header.Set(middleware.CSRFHeaderName, "csrf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

func wantError(t *testing.T, resp *http.Response, body []byte, status int, code, field string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, status, body)
	}
	var e api.Error
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	if e.Error.Code != code {
		t.Fatalf("code = %s, want %s; body %s", e.Error.Code, code, body)
	}
	if field == "" {
		return
	}
	for _, d := range e.Error.Details {
		if d.Field == field {
			return
		}
	}
	t.Fatalf("details = %+v, want field %q", e.Error.Details, field)
}

func TestGetMyOrganizationHandler(t *testing.T) {
	srv := newKycServer(t, &fakeKyc{profile: testProfile()})
	resp, body := borrowerRequest(t, srv, http.MethodGet, "/api/v1/organizations/me", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body %s", resp.StatusCode, body)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["beneficiaries"]) != "[]" {
		t.Fatalf("beneficiaries = %s, want []", got["beneficiaries"])
	}
	if string(got["kyc_status"]) != `"draft"` {
		t.Fatalf("kyc_status = %s", got["kyc_status"])
	}
}

func TestUpdateMyOrganizationHandler(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		wantBeneficiaries *int // nil — список не передан в сервис
		wantLEI           *string
	}{
		{name: "beneficiaries не передан", body: `{"name":"ООО Альфа Плюс"}`},
		{name: "beneficiaries: []", body: `{"beneficiaries":[]}`, wantBeneficiaries: ptr(0)},
		{name: "два бенефициара", body: `{"beneficiaries":[{"full_name":"Иванов И.И.","citizenship_country_code":"RU","ownership_percent":"60.00"},{"full_name":"Петров П.П.","citizenship_country_code":"RU","ownership_percent":"40"}]}`, wantBeneficiaries: ptr(2)},
		{name: `lei: "" — очистить`, body: `{"lei":""}`, wantLEI: ptr("")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kyc := &fakeKyc{profile: testProfile()}
			srv := newKycServer(t, kyc)
			resp, body := borrowerRequest(t, srv, http.MethodPatch, "/api/v1/organizations/me", tt.body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d; body %s", resp.StatusCode, body)
			}
			in := kyc.gotUpdate
			if in == nil {
				t.Fatal("сервис не вызван")
			}
			switch {
			case tt.wantBeneficiaries == nil && in.Beneficiaries != nil:
				t.Fatalf("Beneficiaries = %v, want nil", *in.Beneficiaries)
			case tt.wantBeneficiaries != nil && (in.Beneficiaries == nil || len(*in.Beneficiaries) != *tt.wantBeneficiaries):
				t.Fatalf("Beneficiaries = %v, want %d", in.Beneficiaries, *tt.wantBeneficiaries)
			}
			if tt.wantLEI != nil && (in.LEI == nil || *in.LEI != *tt.wantLEI) {
				t.Fatalf("LEI = %v, want %q", in.LEI, *tt.wantLEI)
			}
		})
	}
}

// formPart — поле multipart-формы; file != nil — файловая часть.
type formPart struct {
	name  string
	value string
	file  []byte
}

func multipartBody(t *testing.T, parts ...formPart) (string, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, p := range parts {
		if p.file != nil {
			fw, err := w.CreateFormFile(p.name, "устав.pdf")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fw.Write(p.file)
			continue
		}
		_ = w.WriteField(p.name, p.value)
	}
	_ = w.Close()
	return w.FormDataContentType(), &buf
}

func TestUploadMyDocumentHandler(t *testing.T) {
	const uploadPath = "/api/v1/organizations/me/documents"
	pdf := []byte("%PDF-1.4 test")
	docType := formPart{name: "doc_type", value: "charter"}
	file := formPart{name: "file", file: pdf}

	t.Run("успех — 201", func(t *testing.T) {
		kyc := &fakeKyc{}
		ctype, body := multipartBody(t, docType, file)
		resp, raw := sessionRequest(t, newKycServer(t, kyc), "borrower-draft", http.MethodPost, uploadPath, ctype, body)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d; body %s", resp.StatusCode, raw)
		}
		if kyc.gotDocType != repository.KycDocumentTypeCharter || kyc.gotFileName != "устав.pdf" || kyc.gotSize != int64(len(pdf)) {
			t.Fatalf("сервис получил %s, %q, %d байт", kyc.gotDocType, kyc.gotFileName, kyc.gotSize)
		}
	})

	tests := []struct {
		name   string
		parts  []formPart
		ctype  string // вместо multipart
		svcErr error
		status int
		code   string
		field  string
	}{
		{name: "без file", parts: []formPart{docType}, status: 400, code: "VALIDATION_ERROR", field: "file"},
		{name: "file раньше doc_type", parts: []formPart{file, docType}, status: 400, code: "VALIDATION_ERROR", field: "doc_type"},
		{name: "doc_type=passport", parts: []formPart{{name: "doc_type", value: "passport"}, file}, status: 400, code: "VALIDATION_ERROR", field: "doc_type"},
		{name: "тело 10 МБ + 100 КБ", parts: []formPart{docType, {name: "file", file: bytes.Repeat([]byte{'a'}, 10<<20+100<<10)}}, status: 413, code: "FILE_TOO_LARGE"},
		{name: "сервис: файл больше 10 МБ", parts: []formPart{docType, file}, svcErr: apperr.FileTooLarge(), status: 413, code: "FILE_TOO_LARGE"},
		{name: "сервис: InvalidTransition", parts: []formPart{docType, file}, svcErr: apperr.New(apperr.CodeInvalidTransition, "нельзя"), status: 409, code: "INVALID_STATUS_TRANSITION"},
		{name: "Content-Type: application/json", ctype: "application/json", status: 400, code: "VALIDATION_ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kyc := &fakeKyc{err: tt.svcErr}
			ctype, body := multipartBody(t, tt.parts...)
			if tt.ctype != "" {
				ctype, body = tt.ctype, bytes.NewBufferString(`{"doc_type":"charter"}`)
			}
			resp, raw := sessionRequest(t, newKycServer(t, kyc), "borrower-draft", http.MethodPost, uploadPath, ctype, body)
			wantError(t, resp, raw, tt.status, tt.code, tt.field)
		})
	}
}

func TestDownloadDocumentHandler(t *testing.T) {
	path := "/api/v1/documents/" + uuid.NewString() + "/file"

	t.Run("200 — файл и заголовки", func(t *testing.T) {
		kyc := &fakeKyc{file: service.DocumentFile{
			Body: io.NopCloser(strings.NewReader("%PDF-1.4")), Size: 8,
			FileName: `устав "ООО".pdf`, MimeType: "application/pdf",
		}}
		resp, raw := sessionRequest(t, newKycServer(t, kyc), "admin", http.MethodGet, path, "", nil)
		if resp.StatusCode != http.StatusOK || string(raw) != "%PDF-1.4" {
			t.Fatalf("status = %d; body %q", resp.StatusCode, raw)
		}
		cd := resp.Header.Get("Content-Disposition")
		if !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, "filename*=utf-8''") {
			t.Fatalf("Content-Disposition = %q", cd)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Content-Length") != "8" {
			t.Fatalf("headers = %v", resp.Header)
		}
	})

	t.Run("чужой — 404", func(t *testing.T) {
		kyc := &fakeKyc{err: apperr.NotFound("Документ не найден")}
		resp, raw := sessionRequest(t, newKycServer(t, kyc), "borrower-draft", http.MethodGet, path, "", nil)
		wantError(t, resp, raw, 404, "NOT_FOUND", "")
	})
}

func TestMyDocumentsHandlers(t *testing.T) {
	t.Run("список без документов — []", func(t *testing.T) {
		resp, raw := borrowerRequest(t, newKycServer(t, &fakeKyc{}), http.MethodGet, "/api/v1/organizations/me/documents", "")
		if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(raw)) != "[]" {
			t.Fatalf("status = %d; body %s", resp.StatusCode, raw)
		}
	})

	t.Run("удаление — 204", func(t *testing.T) {
		kyc := &fakeKyc{}
		id := uuid.New()
		resp, raw := borrowerRequest(t, newKycServer(t, kyc), http.MethodDelete, "/api/v1/organizations/me/documents/"+id.String(), "")
		if resp.StatusCode != http.StatusNoContent || kyc.deletedID != id {
			t.Fatalf("status = %d, deleted %s; body %s", resp.StatusCode, kyc.deletedID, raw)
		}
	})
}

func TestSubmitKYCHandler(t *testing.T) {
	t.Run("200 — профиль в under_review", func(t *testing.T) {
		profile := testProfile()
		profile.Org.KycStatus = "under_review"
		resp, raw := borrowerRequest(t, newKycServer(t, &fakeKyc{profile: profile}), http.MethodPost, "/api/v1/organizations/me/kyc/submit", "")
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"kyc_status":"under_review"`) {
			t.Fatalf("status = %d; body %s", resp.StatusCode, raw)
		}
	})

	t.Run("нет документов — 400 с details", func(t *testing.T) {
		kyc := &fakeKyc{err: apperr.Validation("нет", apperr.FieldError{Field: "documents", Message: "нет документа: charter"})}
		resp, raw := borrowerRequest(t, newKycServer(t, kyc), http.MethodPost, "/api/v1/organizations/me/kyc/submit", "")
		wantError(t, resp, raw, 400, "VALIDATION_ERROR", "documents")
	})
}

func ptr[T any](v T) *T { return &v }
