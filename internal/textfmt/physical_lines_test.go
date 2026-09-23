package textfmt

import (
	"reflect"
	"testing"
)

func TestPhysicalLines(t *testing.T) {
	for _, tc := range []struct {
		content string
		want    []string
	}{
		{"", nil}, {"a", []string{"a"}}, {"a\n", []string{"a"}},
		{"\n", []string{""}}, {"\n\n", []string{"", ""}},
		{"a\n\n", []string{"a", ""}}, {"a\r\n\r\n", []string{"a\r", "\r"}},
		{" a \r\nβ", []string{" a \r", "β"}}, {"a\rb", []string{"a\rb"}},
	} {
		if got := PhysicalLines(tc.content); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("PhysicalLines(%q)=%q, want %q", tc.content, got, tc.want)
		}
	}
}
