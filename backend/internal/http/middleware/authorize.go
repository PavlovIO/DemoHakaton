package middleware

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

// Расширения операции в спеке, из которых строится AccessRule.
const (
	extRoles       = "x-roles"
	extKYCRequired = "x-kyc-required"
)

// AccessRule — кто может вызвать операцию: из security, x-roles и x-kyc-required спеки.
type AccessRule struct {
	Public      bool     // security: []
	Roles       []string // x-roles
	KYCRequired bool     // x-kyc-required
}

// AccessRules — правила доступа по operationId, прочитанные из спеки.
type AccessRules struct {
	byOperation map[string]AccessRule
}

// NewAccessRules разбирает спеку один раз при старте. Ошибка = баг в спеке, сервер не стартует.
func NewAccessRules(doc *openapi3.T) (*AccessRules, error) {
	rules := &AccessRules{byOperation: make(map[string]AccessRule)}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			rule, err := accessRuleOf(op)
			if err != nil {
				return nil, fmt.Errorf("%s %s (%s): %w", method, path, op.OperationID, err)
			}
			rules.byOperation[op.OperationID] = rule
		}
	}
	return rules, nil
}

// Rule — правило операции (для тестов и отладки).
func (a *AccessRules) Rule(operationID string) (AccessRule, bool) {
	rule, ok := a.byOperation[operationID]
	return rule, ok
}

func accessRuleOf(op *openapi3.Operation) (AccessRule, error) {
	// Security == nil — операция наследует глобальный security: [cookieAuth];
	// не-nil пустой список — security: [], операция публичная.
	if op.Security != nil && len(*op.Security) == 0 {
		return AccessRule{Public: true}, nil
	}
	roles, err := rolesOf(op)
	if err != nil {
		return AccessRule{}, err
	}
	kyc, err := kycRequiredOf(op)
	if err != nil {
		return AccessRule{}, err
	}
	return AccessRule{Roles: roles, KYCRequired: kyc}, nil
}

// rolesOf читает x-roles: из YAML список приходит как []any.
func rolesOf(op *openapi3.Operation) ([]string, error) {
	raw, ok := op.Extensions[extRoles].([]any)
	if !ok {
		return nil, fmt.Errorf("%s должен быть списком ролей", extRoles)
	}
	roles := make([]string, 0, len(raw))
	for _, r := range raw {
		role, ok := r.(string)
		if !ok {
			return nil, fmt.Errorf("%s: роль %v не строка", extRoles, r)
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func kycRequiredOf(op *openapi3.Operation) (bool, error) {
	raw, ok := op.Extensions[extKYCRequired]
	if !ok {
		return false, nil
	}
	kyc, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("%s должен быть bool, а не %T", extKYCRequired, raw)
	}
	return kyc, nil
}
