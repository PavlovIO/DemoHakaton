package authapi

import "net/http"

// Register вешает операции тега на общий mux: strict-обёртка над si + сгенерированный роутинг.
// reqErr — ошибки разбора запроса (параметры, тело), respErr — ошибки, которые вернул si.
// Файл одинаков во всех <tag>api, кроме строки package.
func Register(mux *http.ServeMux, si StrictServerInterface, baseURL string, reqErr, respErr func(http.ResponseWriter, *http.Request, error)) {
	strict := NewStrictHandlerWithOptions(si, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  reqErr,
		ResponseErrorHandlerFunc: respErr,
	})
	HandlerWithOptions(strict, StdHTTPServerOptions{
		BaseURL:          baseURL,
		BaseRouter:       mux,
		ErrorHandlerFunc: reqErr,
	})
}
