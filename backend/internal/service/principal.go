package service

import "github.com/google/uuid"

// Principal содержит идентификаторы пользователя и организации для контекста запроса.
type Principal struct {
	UserID uuid.UUID
	OrgID  *uuid.UUID
	Role   string
}
