// Package dealsapi — сгенерированный HTTP-сервер операций тега deals из api/openapi.yaml
// (ADR-0003). Модели — общий пакет api, реализация — internal/http/handler.
// server.gen.go не редактировать: make gen.
package dealsapi

//go:generate go tool oapi-codegen -config ../../../../oapi-codegen/server.yaml -package dealsapi -include-tags deals -o server.gen.go ../../../../../api/openapi.yaml
