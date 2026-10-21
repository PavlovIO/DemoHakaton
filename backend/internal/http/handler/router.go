package handler

import (
	"net/http"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/api"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/http/middleware"
)

// BaseURL — префикс API (servers[0].url в api/openapi.yaml).
const BaseURL = "/api/v1"

// NewRouter собирает http.Handler всего API: сгенерированный роутинг + strict-обёртка
// + единый формат ошибок на всех уровнях + общие middleware.
func NewRouter(h *Handler) http.Handler {
	strict := api.NewStrictHandlerWithOptions(h, nil, api.StrictHTTPServerOptions{
		// Битое тело запроса (JSON не парсится).
		RequestErrorHandlerFunc: requestError,
		// Любая ошибка, которую вернул метод Handler.
		ResponseErrorHandlerFunc: WriteError,
	})

	mux := http.NewServeMux()
	// Всё, что не совпало ни с одной операцией спеки, — JSON 404 вместо text/plain от ServeMux.
	// Обычно сюда не доходит: validateRequests отдаёт 404 раньше; это подстраховка.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, apperr.NotFound("Ресурс не найден"))
	})

	routes := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		BaseURL:    BaseURL,
		BaseRouter: mux,
		// Неверный path/query-параметр (например, не-UUID в {applicationId}).
		ErrorHandlerFunc: requestError,
	})

	return middleware.Chain(routes,
		middleware.RequestLog, // самый внешний: логируем и отбитые запросы
		middleware.Recover(func(w http.ResponseWriter, r *http.Request) { // паника в валидаторе -> 500 в формате Error
			WriteError(w, r, apperr.New(apperr.CodeInternal, "panic"))
		}),
		middleware.MaxBody(bodyLimit), // до валидатора: он читает тело целиком в память
		validateRequests(),            // внутри Recover, снаружи роутинга спеки
	)
}
