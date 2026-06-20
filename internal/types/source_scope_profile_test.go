package types

import "testing"

func TestSourceScopeAllowsPathRole(t *testing.T) {
	cases := []struct {
		scope SourceScope
		role  SourcePathRole
		want  bool
	}{
		{SourceScopeProduction, SourcePathRoleProduction, true},
		{SourceScopeProduction, SourcePathRoleTest, false},
		{SourceScopeTest, SourcePathRoleTest, true},
		{SourceScopeTest, SourcePathRoleProduction, false},
		{SourceScopeDocumentation, SourcePathRoleDocumentation, true},
		{SourceScopeAuxiliary, SourcePathRoleFixture, true},
		{SourceScopeAuxiliary, SourcePathRoleThirdParty, true},
		{SourceScopeAuxiliary, SourcePathRoleVendor, true},
		{SourceScopeAuxiliary, SourcePathRoleGenerated, true},
		{SourceScopeAuxiliary, SourcePathRoleProduction, false},
		{SourceScopeAll, SourcePathRoleExample, true},
	}
	for _, tc := range cases {
		if got := SourceScopeAllowsPathRole(tc.scope, tc.role); got != tc.want {
			t.Fatalf("SourceScopeAllowsPathRole(%q, %q) = %v, want %v", tc.scope, tc.role, got, tc.want)
		}
	}
}

func TestClassifySourcePathRole_RepositoryOwnedAuxiliaryClasses(t *testing.T) {
	tests := []struct {
		path string
		want SourcePathRole
	}{
		{"internal/thirdparty/tree-sitter-arkts/corpus/sources/entry.ets", SourcePathRoleThirdParty},
		{"vendor/google/type/date.proto", SourcePathRoleVendor},
		{"build/generated/package.json", SourcePathRoleGenerated},
		{"src/generated/foo.ts", SourcePathRoleGenerated},
	}
	for _, tc := range tests {
		if got := ClassifySourcePathRole(tc.path); got != tc.want {
			t.Fatalf("ClassifySourcePathRole(%q) = %q, want %q", tc.path, got, tc.want)
		}
		if !SourcePathRoleIsAuxiliary(tc.want) {
			t.Fatalf("%q should be auxiliary", tc.want)
		}
	}
}

func TestSourceScopeProfileAllowsAuxiliaryPrincipal(t *testing.T) {
	cases := []struct {
		name string
		in   *SourceScopeProfile
		want bool
	}{
		{"nil", nil, false},
		{"production", &SourceScopeProfile{RequestedScope: SourceScopeProduction}, false},
		{"test", &SourceScopeProfile{RequestedScope: SourceScopeTest}, true},
		{"documentation", &SourceScopeProfile{RequestedScope: SourceScopeDocumentation}, true},
		{"explicit flag", &SourceScopeProfile{RequestedScope: SourceScopeUnknown, IncludeAuxiliaryAsPrincipal: true}, true},
	}
	for _, tc := range cases {
		if got := tc.in.AllowsAuxiliaryPrincipal(); got != tc.want {
			t.Fatalf("%s: AllowsAuxiliaryPrincipal() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
