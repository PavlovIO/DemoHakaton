package main

import (
	"strings"
	"testing"
)

func TestIsLocalDBHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		{"/var/run/postgresql", true},
		{"postgres", true}, // сервис docker-compose
		{"db.internal.example.com", false},
		{"10.0.0.5", false},
		{"2001:db8::1", false},
	}
	for _, tt := range tests {
		if got := isLocalDBHost(tt.host); got != tt.want {
			t.Errorf("isLocalDBHost(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestRoleWarnings(t *testing.T) {
	tests := []struct {
		name     string
		role     dbRole
		maxConns int32
		want     []string // подстроки предупреждений по порядку
	}{
		{name: "lp_app", role: dbRole{connLimit: 50}, maxConns: 8},
		{name: "superuser", role: dbRole{superuser: true, schemaOwner: true, connLimit: -1}, maxConns: 8, want: []string{"superuser"}},
		{name: "member of lp_migrator", role: dbRole{rewritesAudit: true, connLimit: 50}, maxConns: 8, want: []string{"schema owner"}},
		{name: "owner of empty schema", role: dbRole{schemaOwner: true, connLimit: -1}, maxConns: 8, want: []string{"schema owner"}},
		{name: "pool over limit", role: dbRole{connLimit: 50}, maxConns: 64, want: []string{"exceeds CONNECTION LIMIT 50"}},
		{name: "pool at limit", role: dbRole{connLimit: 50}, maxConns: 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roleWarnings(tt.role, tt.maxConns)
			if len(got) != len(tt.want) {
				t.Fatalf("warnings = %q, want %d matching %q", got, len(tt.want), tt.want)
			}
			for i, sub := range tt.want {
				if !strings.Contains(got[i], sub) {
					t.Errorf("warning %d = %q, want it to contain %q", i, got[i], sub)
				}
			}
		})
	}
}
