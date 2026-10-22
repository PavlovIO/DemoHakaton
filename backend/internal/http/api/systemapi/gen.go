// Package systemapi — сгенерированный HTTP-сервер операций тега system из api/openapi.yaml
// (ADR-0003). Модели — общий пакет api, реализация — internal/http/handler.
// server.gen.go не редактировать: make gen.
package systemapi

//go:generate go tool oapi-codegen -config ../../../../oapi-codegen/server.yaml -package systemapi -include-tags system -o server.gen.go ../../../../../api/openapi.yaml
