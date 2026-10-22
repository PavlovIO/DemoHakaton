// Package notificationsapi — сгенерированный HTTP-сервер операций тега notifications из api/openapi.yaml
// (ADR-0003). Модели — общий пакет api, реализация — internal/http/handler.
// server.gen.go не редактировать: make gen.
package notificationsapi

//go:generate go tool oapi-codegen -config ../../../../oapi-codegen/server.yaml -package notificationsapi -include-tags notifications -o server.gen.go ../../../../../api/openapi.yaml
