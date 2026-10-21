// Package openapi отдаёт контракт api/openapi.yaml, встроенный в бинарник.
//
// openapi.yaml в этом каталоге — копия api/openapi.yaml, которую пишет specbundle
// (//go:embed не умеет ../, а модуль Go — backend/). Артефакт побайтово равен файлу,
// без deflate и зависимости от версии тулчейна, как было у embedded-spec oapi-codegen.
package openapi

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
)

// Spec — сырой YAML контракта (для отдачи клиентам, Swagger UI и т.п.).
//
//go:embed openapi.yaml
var Spec []byte

var load = sync.OnceValues(func() (*openapi3.T, error) {
	doc, err := openapi3.NewLoader().LoadFromData(Spec)
	if err != nil {
		return nil, fmt.Errorf("parse embedded openapi.yaml: %w", err)
	}
	// В спеке servers[0].url = http://localhost:8080/api/v1. kin-openapi сверял бы по нему Host,
	// и запрос на 127.0.0.1 из теста не нашёл бы ни одной операции. Префикс /api/v1 снимает
	// middleware валидации (Options.Prefix). Обнуляем здесь, а не через DoNotValidateServers,
	// чтобы мутация была в одном явном месте.
	doc.Servers = nil
	return doc, nil
})

// Doc возвращает разобранную спеку; разбор выполняется один раз на процесс.
func Doc() (*openapi3.T, error) { return load() }

// MustDoc — для старта сервера: битая встроенная спека это баг сборки, а не рантайма.
func MustDoc() *openapi3.T {
	doc, err := load()
	if err != nil {
		panic(err)
	}
	return doc
}

func init() {
	// kin-openapi из коробки проверяет только byte, date, date-time, int32, int64.
	// Готовая FormatOfStringForUUIDOfRFC4122 требует версию 1–5 и забраковала бы UUIDv7,
	// поэтому своя проверка формы без версии. DefineStringFormatValidator пишет
	// в глобальную мапу — только здесь и только в init().
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`))
}
