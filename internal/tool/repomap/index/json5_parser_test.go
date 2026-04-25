package index

import (
	"testing"
)

func TestParseJSON5_Basic(t *testing.T) {
	src := []byte(`{
    // This is a comment
    name: "entry",
    version: '1.0.0',
    /* block comment */
    dependencies: {
        "libfoo": "1.2.3",
        libbar: "^2.0",   // trailing comma next
    },
    main: "./Index.ets",
    trailing: true,
}`)
	var out map[string]any
	if err := ParseJSON5(src, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out["name"] != "entry" { t.Errorf("name: %v", out["name"]) }
	if out["version"] != "1.0.0" { t.Errorf("version: %v", out["version"]) }
	deps, ok := out["dependencies"].(map[string]any)
	if !ok || deps["libfoo"] != "1.2.3" || deps["libbar"] != "^2.0" {
		t.Errorf("deps: %v", out["dependencies"])
	}
	if out["trailing"] != true { t.Errorf("trailing bool: %v", out["trailing"]) }
}

func TestParseJSON5_NumbersAndNulls(t *testing.T) {
	src := []byte(`{ a: 42, b: -3.14, c: 0xff, d: null, e: true, f: false }`)
	var out map[string]any
	if err := ParseJSON5(src, &out); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out["a"] != float64(42) { t.Errorf("a: %v", out["a"]) }
	if out["b"] != float64(-3.14) { t.Errorf("b: %v", out["b"]) }
	if out["c"] != float64(255) { t.Errorf("c: %v", out["c"]) }
	if out["d"] != nil { t.Errorf("d: %v", out["d"]) }
	if out["e"] != true || out["f"] != false { t.Errorf("e/f: %v %v", out["e"], out["f"]) }
}
