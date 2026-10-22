// Package chatapi — сгенерированный HTTP-сервер операций тега chat из api/openapi.yaml
// (ADR-0003). Модели — общий пакет api, реализация — internal/http/handler.
// server.gen.go не редактировать: make gen.
package chatapi

//go:generate go tool oapi-codegen -config ../../../../oapi-codegen/server.yaml -package chatapi -include-tags chat -o server.gen.go ../../../../../api/openapi.yaml
