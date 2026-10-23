package handler

import (
	"fmt"
	"net/http"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/adminapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/applicationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/authapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/chatapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/dealsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/dictionariesapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/notificationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/offersapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/organizationsapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api/systemapi"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/openapi"
)

// BaseURL — префикс API (servers[0].url в api/openapi.yaml).
const BaseURL = "/api/v1"

// NewRouter собирает http.Handler всего API: сгенерированный роутинг каждого тега (routes)
// + strict-обёртка + единый формат ошибок на всех уровнях + общие middleware.
func NewRouter(h *Handler) http.Handler {
	// Без сервиса аутентификации все защищённые ручки молча отвечали бы 401 — лучше не стартовать.
	if h.auth == nil {
		panic("handler: Deps.Auth is nil")
	}
	// Ошибка в правилах доступа — баг в спеке: сервер не должен стартовать с открытой ручкой.
	rules, err := middleware.NewAccessRules(openapi.MustDoc())
	if err != nil {
		panic(fmt.Sprintf("access rules: %v", err))
	}

	return middleware.Chain(routes(h),
		middleware.RequestLog, // самый внешний: логируем и отбитые запросы
		middleware.Recover(func(w http.ResponseWriter, r *http.Request) { // паника в валидаторе -> 500 в формате Error
			WriteError(w, r, apperr.New(apperr.CodeInternal, "panic"))
		}),
		middleware.SecurityHeaders(),
		middleware.MaxBody(bodyLimit), // до валидатора: он читает тело целиком в память
		uploadDeadlines,               // загрузке документа — 60 с вместо ReadTimeout сервера
		middleware.WithRequestMeta(),
		middleware.Authenticate(h.auth, WriteError),      // мягкий: только кладёт principal
		middleware.Authorize(rules, BaseURL, WriteError), // 401 / 403 по x-roles и x-kyc-required
		middleware.CSRF(WriteError),                      // после Authorize: берёт его правило операции
		validateRequests(),                               // внутри Recover, снаружи роутинга спеки
	)
}

// routes — роутинг спеки без middleware: по пакету на тег на общем mux.
func routes(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	// Всё, что не совпало ни с одной операцией спеки, — JSON 404 вместо text/plain от ServeMux.
	// Обычно сюда не доходит: validateRequests отдаёт 404 раньше; это подстраховка.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, apperr.NotFound("Ресурс не найден"))
	})

	// Пакет на тег (ADR-0003): каждый вешает свои операции на общий mux.
	// requestError — неверный path/query-параметр или битое тело (JSON не парсится),
	// WriteError — любая ошибка, которую вернул метод Handler.
	// Новый тег без строки здесь не попадёт в роутинг — это ловит TestEveryOperationIsRouted.
	authapi.Register(mux, h, BaseURL, requestError, WriteError)
	organizationsapi.Register(mux, h, BaseURL, requestError, WriteError)
	dictionariesapi.Register(mux, h, BaseURL, requestError, WriteError)
	applicationsapi.Register(mux, h, BaseURL, requestError, WriteError)
	offersapi.Register(mux, h, BaseURL, requestError, WriteError)
	dealsapi.Register(mux, h, BaseURL, requestError, WriteError)
	chatapi.Register(mux, h, BaseURL, requestError, WriteError)
	notificationsapi.Register(mux, h, BaseURL, requestError, WriteError)
	adminapi.Register(mux, h, BaseURL, requestError, WriteError)
	systemapi.Register(mux, h, BaseURL, requestError, WriteError)

	return mux
}
