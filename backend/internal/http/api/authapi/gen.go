// Package authapi — сгенерированный HTTP-сервер операций тега auth из api/openapi.yaml
// (ADR-0003). Модели — общий пакет api, реализация — internal/http/handler.
// server.gen.go не редактировать: make gen.
package authapi

//go:generate go tool oapi-codegen -config ../../../../oapi-codegen/server.yaml -package authapi -include-tags auth -o server.gen.go ../../../../../api/openapi.yaml
