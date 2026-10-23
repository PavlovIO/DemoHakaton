package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/openapi"
)

// Лимиты тела запроса: валидатор читает тело целиком в память (см. middleware.MaxBody).
const (
	maxBodyDefault int64 = 1 << 20 // 1 МБ — JSON-тела
	// 10 МБ файла по спеке плюс запас на multipart-обёртку и остальные поля формы:
	// лимит стоит на всём теле, а не на файле. Размер самого файла проверяет сервис;
	// превышение любого из лимитов — 413 FILE_TOO_LARGE (UploadMyDocument).
	maxBodyUpload int64 = 10<<20 + 64<<10
)

// uploadTimeout — сколько можно принимать загрузку. ReadTimeout сервера (15 с) действует
// на всё тело: 10 МБ за 15 с — это ≥ 5,6 Мбит/с, на медленном канале загрузка оборвалась бы.
const uploadTimeout = 60 * time.Second

// uploadResponseTimeout — запас на запись ответа после приёма файла: WriteTimeout
// сервера отсчитывается от начала запроса и истёк бы раньше, чем ушёл ответ 201.
const uploadResponseTimeout = 10 * time.Second

// uploadDeadlines продлевает дедлайны чтения и записи соединения только для загрузки
// документа; остальные запросы живут по таймаутам сервера.
func uploadDeadlines(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUpload(r) {
			rc := http.NewResponseController(w)
			now := time.Now()
			// Ошибка — ResponseWriter без поддержки дедлайнов (тесты): остаются таймауты сервера.
			_ = rc.SetReadDeadline(now.Add(uploadTimeout))
			_ = rc.SetWriteDeadline(now.Add(uploadTimeout + uploadResponseTimeout))
		}
		next.ServeHTTP(w, r)
	})
}

// isUpload — POST /organizations/me/documents (multipart с файлом).
func isUpload(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == BaseURL+"/organizations/me/documents"
}

func bodyLimit(r *http.Request) int64 {
	if isUpload(r) {
		return maxBodyUpload
	}
	return maxBodyDefault
}

// validateRequests — middleware, проверяющая запрос по встроенной спеке: путь, метод,
// параметры (required, enum, pattern, minimum…) и тело (JSON-схема). Strict server
// oapi-codegen этого не делает by design. Четыре настройки ниже обязательны — без любой
// из них ломается всё сразу, а не по чуть-чуть.
func validateRequests() func(http.Handler) http.Handler {
	return nethttpmiddleware.OapiRequestValidatorWithOptions(openapi.MustDoc(), &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			// В спеке глобальный security: [cookieAuth]; при nil-функции openapi3filter
			// отдаёт SecurityRequirementsError на каждый запрос (401 на всё).
			// Сессию и роли проверяют свои middleware раньше (Authenticate, Authorize).
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
			// Все ошибки формы, а не первая: одна запись на поле в error.details.
			MultiError: true,
		},
		// В спеке пути без /api/v1 (он в servers[0].url, а Servers обнулены в internal/openapi).
		// Без Prefix — 404 на каждый запрос.
		Prefix:                BaseURL,
		SilenceServersWarning: true,
		ErrorHandlerWithOpts:  validationError,
		Skipper:               skipValidation,
	})
}

// skipValidation — запросы, которые валидатор не трогает:
//   - загрузка файла: kin-openapi читает multipart целиком в память
//     (тело в []byte плюс содержимое файла в string) — ~20 МБ на запрос;
//   - HEAD: ServeMux сопоставляет его с GET-паттернами, а роутер валидатора (gorillamux) —
//     нет, и HEAD /healthz получал бы 404. Тела у HEAD нет, остальное проверит биндер.
func skipValidation(r *http.Request) bool {
	return isUpload(r) || r.Method == http.MethodHead
}

// validationError переводит ошибку валидатора в единый формат Error.
// opts.StatusCode сознательно игнорируется: статус определяет statusByCode по коду
// apperr (единственный источник правды в errors.go) — не добавляйте сюда opts.StatusCode.
func validationError(_ context.Context, err error, w http.ResponseWriter, r *http.Request, opts nethttpmiddleware.ErrorHandlerOpts) {
	if opts.MatchedRoute == nil {
		// Путь или метод не из спеки — ровно как catch-all в routes(): 404, а не 400.
		WriteError(w, r, apperr.NotFound("Ресурс не найден"))
		return
	}
	WriteError(w, r, specError(err))
}

// specError раскладывает ошибку openapi3filter на apperr с details по полям.
func specError(err error) error {
	// Проверку security openapi3filter делает первой и перед ней читает тело целиком —
	// превышение лимита MaxBody приходит завёрнутым в SecurityRequirementsError, а не как
	// ошибка тела. Поэтому лимит проверяем раньше всего.
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		e := apperr.Validation("Запрос не соответствует контракту", fieldErrors(maxBytes)...)
		e.Err = err
		return e
	}

	var secErr *openapi3filter.SecurityRequirementsError
	if errors.As(err, &secErr) {
		// Недостижимо с NoopAuthenticationFunc, но маппинг код -> статус должен остаться верным.
		return apperr.Wrap(apperr.CodeUnauthorized, "Требуется аутентификация", err)
	}

	var details []apperr.FieldError
	var multi openapi3.MultiError
	if errors.As(err, &multi) {
		for _, e := range multi {
			details = append(details, fieldErrors(e)...)
		}
	} else {
		details = append(details, fieldErrors(err)...)
	}
	e := apperr.Validation("Запрос не соответствует контракту", details...)
	e.Err = err // полный текст — только в лог
	return e
}

// fieldErrors — записи details для одной ошибки валидатора: откуда она
// (параметр / путь в теле) и короткое сообщение. Для тела с несколькими нарушениями
// openapi3filter кладёт в RequestError.Err вложенный MultiError — по записи на каждое.
func fieldErrors(err error) []apperr.FieldError {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return []apperr.FieldError{{Field: "body", Message: "тело запроса слишком большое"}}
	}

	var reqErr *openapi3filter.RequestError
	if !errors.As(err, &reqErr) {
		return []apperr.FieldError{{Field: "body", Message: "некорректный запрос"}}
	}

	schemaErrs := collectSchemaErrors(reqErr.Err)

	if reqErr.Parameter != nil {
		msg := "некорректное значение"
		if len(schemaErrs) > 0 {
			msg = schemaMessage(schemaErrs[0])
		}
		return []apperr.FieldError{{Field: reqErr.Parameter.Name, Message: msg}}
	}

	if len(schemaErrs) == 0 {
		return []apperr.FieldError{{Field: "body", Message: "некорректное тело запроса"}}
	}
	out := make([]apperr.FieldError, 0, len(schemaErrs))
	for _, se := range schemaErrs {
		// Путь внутри тела: beneficiaries.0.full_name; пусто — ошибка на корне тела.
		field := strings.Join(se.JSONPointer(), ".")
		if field == "" {
			field = "body"
		}
		out = append(out, apperr.FieldError{Field: field, Message: schemaMessage(se)})
	}
	return out
}

// collectSchemaErrors разворачивает вложенные MultiError и собирает все SchemaError.
func collectSchemaErrors(err error) []*openapi3.SchemaError {
	// Именно утверждение типа, не errors.As: MultiError.As делегирует элементам и вернул бы
	// первый SchemaError вместо самого контейнера.
	if multi, ok := err.(openapi3.MultiError); ok { //nolint:errorlint
		var out []*openapi3.SchemaError
		for _, e := range multi {
			out = append(out, collectSchemaErrors(e)...)
		}
		return out
	}
	var se *openapi3.SchemaError
	if errors.As(err, &se) {
		return []*openapi3.SchemaError{se}
	}
	return nil
}

// schemaMessage — короткий русский текст по тому, какое правило схемы нарушено.
// SchemaError.Reason по контракту kin-openapi не содержит исходное значение,
// так что ввод пользователя в ответ не утечёт и в ветке default.
func schemaMessage(e *openapi3.SchemaError) string {
	switch e.SchemaField {
	case "required":
		return "обязательное поле"
	case "enum":
		return "недопустимое значение"
	case "pattern", "format":
		return "неверный формат"
	case "minLength", "maxLength":
		return "недопустимая длина"
	case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum":
		return "значение вне допустимого диапазона"
	case "type":
		return "неверный тип значения"
	case "minItems", "maxItems":
		return "недопустимое количество элементов"
	default:
		return e.Reason
	}
}
