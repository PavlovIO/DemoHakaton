package handler

import (
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/openapi"
)

// generatedTags — теги, для которых есть пакет internal/http/api/<tag>api (ADR-0003).
// Новый тег в спеке = новый пакет + строка в routes() + строка здесь.
var generatedTags = map[string]bool{
	"auth": true, "organizations": true, "dictionaries": true, "applications": true,
	"offers": true, "deals": true, "chat": true, "notifications": true,
	"admin": true, "system": true,
}

// Сервер генерируется по пакету на тег (-include-tags): операция без тега или с тегом без
// пакета молча пропадёт из роутинга, а с двумя тегами — попадёт в два пакета, и ServeMux
// упадёт с паникой на дубле маршрута.
func TestEveryOperationHasExactlyOneGeneratedTag(t *testing.T) {
	for path, item := range openapi.MustDoc().Paths.Map() {
		for method, op := range item.Operations() {
			if len(op.Tags) != 1 || !generatedTags[op.Tags[0]] {
				t.Errorf("%s %s (%s): теги %v — нужен ровно один из generatedTags", method, path, op.OperationID, op.Tags)
			}
		}
	}
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// Каждая операция спеки зарегистрирована на mux: ловит забытую строку Register в routes().
// Проверяется сам ServeMux, а не HTTP-ответ: через NewRouter многие запросы отбил бы
// валидатор (нет тела, обязательного параметра) раньше, чем дело дошло бы до роутинга.
func TestEveryOperationIsRouted(t *testing.T) {
	mux := routes(New(Deps{}))
	n := 0
	for path, item := range openapi.MustDoc().Paths.Map() {
		for method, op := range item.Operations() {
			n++
			req := httptest.NewRequest(method, BaseURL+pathParam.ReplaceAllString(path, "x"), nil)
			if _, pattern := mux.Handler(req); pattern == "/" {
				t.Errorf("%s %s (%s) не зарегистрирован: попал в fallback 404", method, path, op.OperationID)
			}
		}
	}
	if n == 0 {
		t.Fatal("в спеке нет операций — тест ничего не проверил")
	}
}
