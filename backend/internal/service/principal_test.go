package service

import (
	"testing"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
)

func TestPrincipalRequireRole(t *testing.T) {
	const (
		borrower = repository.UserRoleBorrower
		lender   = repository.UserRoleLender
		admin    = repository.UserRoleAdmin
	)
	tests := []struct {
		name    string
		role    string
		allowed []repository.UserRole
		wantErr bool
	}{
		{name: "роль разрешена", role: "borrower", allowed: []repository.UserRole{borrower}, wantErr: false},
		{name: "роль среди нескольких", role: "lender", allowed: []repository.UserRole{borrower, lender}, wantErr: false},
		{name: "admin там, где нужен admin", role: "admin", allowed: []repository.UserRole{admin}, wantErr: false},
		{name: "чужая роль", role: "lender", allowed: []repository.UserRole{borrower}, wantErr: true},
		{name: "admin в операции компании", role: "admin", allowed: []repository.UserRole{borrower, lender}, wantErr: true},
		{name: "неизвестная роль", role: "superuser", allowed: []repository.UserRole{borrower, lender, admin}, wantErr: true},
		{name: "пустая роль", role: "", allowed: []repository.UserRole{borrower}, wantErr: true},
		{name: "пустой список закрыт", role: "admin", allowed: nil, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Principal{Role: tc.role}.RequireRole(tc.allowed...)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("RequireRole: ошибка %v, ждали nil", err)
				}
				return
			}
			if got := apperr.CodeOf(err); got != apperr.CodeForbidden {
				t.Fatalf("RequireRole: код %q (err=%v), ждали %q", got, err, apperr.CodeForbidden)
			}
		})
	}
}

func TestPrincipalRequireKYCVerified(t *testing.T) {
	tests := []struct {
		kycStatus string
		wantErr   bool
	}{
		{kycStatus: "verified", wantErr: false},
		{kycStatus: "draft", wantErr: true},
		{kycStatus: "under_review", wantErr: true},
		{kycStatus: "rejected", wantErr: true},
		{kycStatus: "", wantErr: true}, // admin: компании и KYC нет
	}
	for _, tc := range tests {
		t.Run(tc.kycStatus, func(t *testing.T) {
			err := Principal{Role: "borrower", KycStatus: tc.kycStatus}.RequireKYCVerified()
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("RequireKYCVerified: ошибка %v, ждали nil", err)
				}
				return
			}
			if got := apperr.CodeOf(err); got != apperr.CodeKYCNotVerified {
				t.Fatalf("RequireKYCVerified: код %q (err=%v), ждали %q", got, err, apperr.CodeKYCNotVerified)
			}
		})
	}
}
