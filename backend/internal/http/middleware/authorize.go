package middleware

import (
	"fmt"
	"slices"

	"github.com/getkin/kin-openapi/openapi3"

	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
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

// knownRoles — значения enum user_role в БД; другой роли в x-roles быть не может.
var knownRoles = map[string]bool{
	string(repository.UserRoleBorrower): true,
	string(repository.UserRoleLender):   true,
	string(repository.UserRoleAdmin):    true,
}

// accessRuleOf строит правило операции и отвергает спеку, в которой доступ описан
// неоднозначно: защищённая операция без ролей открылась бы всем, а не никому.
func accessRuleOf(op *openapi3.Operation) (AccessRule, error) {
	_, hasRoles := op.Extensions[extRoles]
	// Security == nil — операция наследует глобальный security: [cookieAuth];
	// не-nil пустой список — security: [], операция публичная.
	if op.Security != nil && len(*op.Security) == 0 {
		if hasRoles {
			return AccessRule{}, fmt.Errorf("%s у публичной операции (security: [])", extRoles)
		}
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
	// У admin нет организации, а значит и kyc_status: с x-kyc-required он не прошёл бы никогда.
	if kyc && slices.Contains(roles, string(repository.UserRoleAdmin)) {
		return AccessRule{}, fmt.Errorf("%s у операции, доступной admin", extKYCRequired)
	}
	return AccessRule{Roles: roles, KYCRequired: kyc}, nil
}

// rolesOf читает x-roles: из YAML список приходит как []any.
func rolesOf(op *openapi3.Operation) ([]string, error) {
	raw, ok := op.Extensions[extRoles].([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("у защищённой операции %s должен быть непустым списком ролей", extRoles)
	}
	roles := make([]string, 0, len(raw))
	for _, r := range raw {
		role, ok := r.(string)
		if !ok || !knownRoles[role] {
			return nil, fmt.Errorf("%s: неизвестная роль %v", extRoles, r)
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
