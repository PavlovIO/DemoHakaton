// Package api — модели контракта api/openapi.yaml: схемы, параметры (XxxParams),
// тела запросов (XxxJSONRequestBody), enum'ы. Генерируется в models.gen.go (make gen).
//
// Раскладка HTTP-слоя (ADR-0003):
//
//	api/                модели — один пакет на весь контракт: схемы ссылаются друг на друга
//	                    по кругу (organizations ↔ auth ↔ aml), а Go запрещает циклы импортов
//	api/<tag>api/       сервер одного тега: роутинг, разбор параметров, StrictServerInterface,
//	                    XxxRequestObject/XxxResponseObject; модели берёт отсюда (dot-import)
//	handler/<tag>.go    реализация операций тега — пишем руками
//
// Рукописных типов в этот пакет не добавлять: через dot-import они попадают в каждый
// <tag>api и могут столкнуться со сгенерированными именами.
package api
