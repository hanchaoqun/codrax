package index

import "testing"

func TestParseTOMLSubset(t *testing.T) {
	src := []byte(`# cjpm.toml
[package]
name = "demo"
version = "0.1.0"
edition = "1.0"

[dependencies]
libfoo = "1.2.3"
libbar = "^2.0"   # trailing comment

[build]
target = "cjnative"
`)
	out, err := ParseTOMLSubset(src)
	if err != nil { t.Fatal(err) }
	if out["package"]["name"] != "demo" { t.Errorf("package.name=%v", out["package"]["name"]) }
	if out["dependencies"]["libfoo"] != "1.2.3" { t.Errorf("deps.libfoo=%v", out["dependencies"]["libfoo"]) }
	if out["dependencies"]["libbar"] != "^2.0" { t.Errorf("deps.libbar=%v", out["dependencies"]["libbar"]) }
	if out["build"]["target"] != "cjnative" { t.Errorf("build.target=%v", out["build"]["target"]) }
}
