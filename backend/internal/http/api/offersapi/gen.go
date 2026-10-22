// Package offersapi — сгенерированный HTTP-сервер операций тега offers из api/openapi.yaml
// (ADR-0003). Модели — общий пакет api, реализация — internal/http/handler.
// server.gen.go не редактировать: make gen.
package offersapi

//go:generate go tool oapi-codegen -config ../../../../oapi-codegen/server.yaml -package offersapi -include-tags offers -o server.gen.go ../../../../../api/openapi.yaml
