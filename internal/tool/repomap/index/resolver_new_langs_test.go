package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestRubyImportResolver(t *testing.T) {
	files := []*types.FileInfo{
		{RelPath: "lib/app.rb", Language: types.LangRuby},
		{RelPath: "lib/app/models/user.rb", Language: types.LangRuby},
		{RelPath: "spec/support/helper.rb", Language: types.LangRuby},
	}
	g := BuildGraph(t.TempDir(), files)
	r := &rubyImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	cases := []struct {
		label   string
		fi      *types.FileInfo
		imp     types.Import
		want    []string
		wantRsn string
	}{
		{
			label: "load path require resolves under lib",
			fi:    files[0],
			imp:   types.Import{Path: "app/models/user", Raw: `require "app/models/user"`},
			want:  []string{"lib/app/models/user.rb"},
		},
		{
			label: "require_relative resolves from caller dir",
			fi:    files[2],
			imp:   types.Import{Path: "../lib/app", Raw: `require_relative "../lib/app"`},
			want:  []string{"lib/app.rb"},
		},
		{
			label:   "external require stays unresolved",
			fi:      files[0],
			imp:     types.Import{Path: "json", Raw: `require "json"`},
			wantRsn: "ruby_external",
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			before := len(ctx.Unresolved)
			got := r.Resolve(g, c.fi, c.imp, ctx)
			if !sameStringSet(got, c.want) {
				t.Fatalf("Resolve(%q) = %v, want %v", c.imp.Path, got, c.want)
			}
			if c.wantRsn == "" {
				if len(ctx.Unresolved) != before {
					t.Fatalf("unexpected unresolved append: %+v", ctx.Unresolved[before:])
				}
				return
			}
			if len(ctx.Unresolved) != before+1 {
				t.Fatalf("expected one unresolved append, got delta %d", len(ctx.Unresolved)-before)
			}
			if ctx.Unresolved[before].Reason != c.wantRsn {
				t.Fatalf("reason = %q, want %q", ctx.Unresolved[before].Reason, c.wantRsn)
			}
		})
	}
}

func TestSwiftImportResolver(t *testing.T) {
	files := []*types.FileInfo{
		{RelPath: "Sources/Core/Foo.swift", Language: types.LangSwift, Package: "Core"},
		{RelPath: "Sources/Core/Bar.swift", Language: types.LangSwift, Package: "Core"},
		{RelPath: "Sources/App/Main.swift", Language: types.LangSwift, Package: "App"},
	}
	g := BuildGraph(t.TempDir(), files)
	r := &swiftImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	got := r.Resolve(g, files[2], types.Import{Path: "Core"}, ctx)
	want := []string{"Sources/Core/Foo.swift", "Sources/Core/Bar.swift"}
	if !sameStringSet(got, want) {
		t.Fatalf("Resolve(Core) = %v, want %v", got, want)
	}

	before := len(ctx.Unresolved)
	if got := r.Resolve(g, files[2], types.Import{Path: "Foundation"}, ctx); got != nil {
		t.Fatalf("Resolve(Foundation) = %v, want nil", got)
	}
	if len(ctx.Unresolved) != before+1 || ctx.Unresolved[before].Reason != "swift_external" {
		t.Fatalf("unexpected unresolved = %+v", ctx.Unresolved[before:])
	}
}

func TestLuaImportResolver(t *testing.T) {
	files := []*types.FileInfo{
		{RelPath: "lua/app/init.lua", Language: types.LangLua},
		{RelPath: "lua/app/service.lua", Language: types.LangLua},
		{RelPath: "scripts/bootstrap.lua", Language: types.LangLua},
	}
	g := BuildGraph(t.TempDir(), files)
	r := &luaImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	cases := []struct {
		label   string
		fi      *types.FileInfo
		imp     types.Import
		want    []string
		wantRsn string
	}{
		{
			label: "module require resolves dotted path",
			fi:    files[2],
			imp:   types.Import{Path: "app.service", Raw: `require "app.service"`},
			want:  []string{"lua/app/service.lua"},
		},
		{
			label: "dofile resolves relative file path",
			fi:    files[2],
			imp:   types.Import{Path: "../lua/app/init.lua", Raw: `dofile("../lua/app/init.lua")`},
			want:  []string{"lua/app/init.lua"},
		},
		{
			label:   "external module stays unresolved",
			fi:      files[2],
			imp:     types.Import{Path: "socket.http", Raw: `require "socket.http"`},
			wantRsn: "lua_external",
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			before := len(ctx.Unresolved)
			got := r.Resolve(g, c.fi, c.imp, ctx)
			if !sameStringSet(got, c.want) {
				t.Fatalf("Resolve(%q) = %v, want %v", c.imp.Path, got, c.want)
			}
			if c.wantRsn == "" {
				if len(ctx.Unresolved) != before {
					t.Fatalf("unexpected unresolved append: %+v", ctx.Unresolved[before:])
				}
				return
			}
			if len(ctx.Unresolved) != before+1 {
				t.Fatalf("expected one unresolved append, got delta %d", len(ctx.Unresolved)-before)
			}
			if ctx.Unresolved[before].Reason != c.wantRsn {
				t.Fatalf("reason = %q, want %q", ctx.Unresolved[before].Reason, c.wantRsn)
			}
		})
	}
}

func TestProtoImportResolver(t *testing.T) {
	files := []*types.FileInfo{
		{RelPath: "proto/api/service.proto", Language: types.LangProto},
		{RelPath: "proto/common/types.proto", Language: types.LangProto},
		{RelPath: "vendor/google/type/date.proto", Language: types.LangProto},
	}
	g := BuildGraph(t.TempDir(), files)
	r := &protoImportResolver{}
	ctx := newResolverContext(g)
	if err := r.Prepare(g, ctx); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	cases := []struct {
		label   string
		imp     string
		want    []string
		wantRsn string
	}{
		{
			label: "relative import resolves sibling suffix",
			imp:   "../common/types.proto",
			want:  []string{"proto/common/types.proto"},
		},
		{
			label: "include-root import resolves by suffix",
			imp:   "google/type/date.proto",
			want:  []string{"vendor/google/type/date.proto"},
		},
		{
			label:   "external proto stays unresolved",
			imp:     "google/protobuf/empty.proto",
			wantRsn: "proto_external",
		},
	}

	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			before := len(ctx.Unresolved)
			got := r.Resolve(g, files[0], types.Import{Path: c.imp}, ctx)
			if !sameStringSet(got, c.want) {
				t.Fatalf("Resolve(%q) = %v, want %v", c.imp, got, c.want)
			}
			if c.wantRsn == "" {
				if len(ctx.Unresolved) != before {
					t.Fatalf("unexpected unresolved append: %+v", ctx.Unresolved[before:])
				}
				return
			}
			if len(ctx.Unresolved) != before+1 {
				t.Fatalf("expected one unresolved append, got delta %d", len(ctx.Unresolved)-before)
			}
			if ctx.Unresolved[before].Reason != c.wantRsn {
				t.Fatalf("reason = %q, want %q", ctx.Unresolved[before].Reason, c.wantRsn)
			}
		})
	}
}
