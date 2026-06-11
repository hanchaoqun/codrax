package dataquery

import "testing"

// Batch-6 E1: row-level arithmetic over numeric fields — the missing
// typed path for tasks like qty*unit_price.
func TestDeriveArithmeticFields(t *testing.T) {
	row := map[string]string{"qty": "2", "unit_price": "10.5", "fee": "1", "zero": "0", "bad": "n/a"}
	cases := []struct {
		op     string
		fields []string
		want   string
	}{
		{"multiply", []string{"qty", "unit_price"}, "21"},
		{"add", []string{"qty", "fee"}, "3"},
		{"subtract", []string{"unit_price", "fee"}, "9.5"},
		{"divide", []string{"unit_price", "qty"}, "5.25"},
		{"divide", []string{"qty", "zero"}, ""},   // div-by-zero -> empty
		{"multiply", []string{"qty", "bad"}, ""},  // non-numeric -> empty
		{"multiply", []string{"qty", "none"}, ""}, // missing field -> empty
	}
	for _, c := range cases {
		got := deriveArithmeticFields(row, deriveFieldSpec{Operation: c.op, SourceFields: c.fields})
		if got != c.want {
			t.Fatalf("%s(%v) = %q, want %q", c.op, c.fields, got, c.want)
		}
	}
}

func TestDeriveFieldOperationSupported_Arithmetic(t *testing.T) {
	for _, op := range []string{"multiply", "divide", "add", "subtract", "product", "sum_fields"} {
		if !deriveFieldOperationSupported(op) {
			t.Fatalf("operation %q must be supported", op)
		}
	}
}
