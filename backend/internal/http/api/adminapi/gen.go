// Package adminapi — сгенерированный HTTP-сервер операций тега admin из api/openapi.yaml
// (ADR-0003). Модели — общий пакет api, реализация — internal/http/handler.
// server.gen.go не редактировать: make gen.
package adminapi

//go:generate go tool oapi-codegen -config ../../../../oapi-codegen/server.yaml -package adminapi -include-tags admin -o server.gen.go ../../../../../api/openapi.yaml
