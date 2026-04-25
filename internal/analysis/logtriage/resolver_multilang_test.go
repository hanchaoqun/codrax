package logtriage

import "testing"

func TestResolveSwiftFile_ModuleRootTierOne(t *testing.T) {
	cands := []string{
		"Sources/App/Main.swift",
		"Tests/AppTests/Main.swift",
		"misc/Main.swift",
	}
	got := ResolveSwiftFile("App", "Main.swift", cands)
	if len(got) == 0 || got[0] != "Sources/App/Main.swift" {
		t.Fatalf("ResolveSwiftFile tier-1 miss: %v", got)
	}
}

func TestResolveLuaFile_SourceRootTiering(t *testing.T) {
	cands := []string{
		"misc/init.lua",
		"lua/app/init.lua",
		"lib/app/init.lua",
	}
	got := ResolveLuaFile("app", "init.lua", cands)
	if len(got) == 0 || got[0] != "lua/app/init.lua" {
		t.Fatalf("ResolveLuaFile root preference miss: %v", got)
	}
}

func TestResolveProtoFile_PackageTierOne(t *testing.T) {
	cands := []string{
		"vendor/payments/v1/Payment.proto",
		"proto/payments/v1/Payment.proto",
		"schemas/Payment.proto",
	}
	got := ResolveProtoFile("payments.v1", "Payment.proto", cands)
	if len(got) == 0 || got[0] != "proto/payments/v1/Payment.proto" {
		t.Fatalf("ResolveProtoFile tier-1 miss: %v", got)
	}
}

func TestBasenameHelpers_ExtendedLanguages(t *testing.T) {
	if !isSwiftBasename("Foo.swift") || isSwiftBasename("src/Foo.swift") {
		t.Fatal("swift basename helper mismatch")
	}
	if !isLuaBasename("main.lua") || isLuaBasename("lua/main.lua") {
		t.Fatal("lua basename helper mismatch")
	}
	if !isProtoBasename("service.proto") || isProtoBasename("proto/service.proto") {
		t.Fatal("proto basename helper mismatch")
	}
}
