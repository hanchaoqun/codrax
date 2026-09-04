package dataquery

import (
	"errors"
	"testing"
)

// action_output_contract_dependence_test.go — V9-4 (colleague_merge_audit
// §40.56) pin ⑤ (normalizer half) and the typed contract-dependence signal
// the workflow drift guard reads.

// TestNormalizeDataActionForOutputContractIsIdempotentAcrossProjectionMatrix
// walks the closed projection table × the closed format set: a normalized
// action normalizes to itself under the same contract (so the planner gate,
// admission and the executor can each run the normalizer without moving the
// action), and every rejection in the matrix is typed as contract-dependent
// with the judged format.
func TestNormalizeDataActionForOutputContractIsIdempotentAcrossProjectionMatrix(t *testing.T) {
	for _, contract := range DataActionOutputProjectionContracts() {
		for _, format := range DataActionOutputProjectionFormats() {
			action := DataAction{Kind: DataActionAssembleAnswer, Params: map[string]string{"projection": contract.Value}}
			output := OutputContract{Format: format}
			once, err := NormalizeDataActionForOutputContract(action, output)
			if err != nil {
				var paramErr DataActionParamError
				if !errors.As(err, &paramErr) || !paramErr.OutputContractDependent() || paramErr.OutputFormat != format {
					t.Fatalf("projection=%s format=%s: err=%T %v, want a contract-dependent typed rejection naming the judged format", contract.Value, format, err, err)
				}
				if outputFormatIn(format, contract.Formats) {
					t.Fatalf("projection=%s format=%s rejected although the table admits it", contract.Value, format)
				}
				continue
			}
			if len(contract.Formats) > 0 && !outputFormatIn(format, contract.Formats) {
				t.Fatalf("projection=%s format=%s admitted although the table rejects it", contract.Value, format)
			}
			twice, err := NormalizeDataActionForOutputContract(once, output)
			if err != nil || twice.Params["projection"] != once.Params["projection"] || len(twice.Params) != len(once.Params) {
				t.Fatalf("projection=%s format=%s: normalize not idempotent: once=%v twice=%v err=%v", contract.Value, format, once.Params, twice.Params, err)
			}
		}
	}
}

// TestDataActionParamErrorContractDependenceIsPrecise pins the closed set of
// contract-dependent rejections: the output_field default that exists only
// under json_only is contract-dependent when the projection is omitted, an
// explicit non-object projection with output_field is rejected under every
// format (contract-independent), and unknown parameters never carry a
// format.
func TestDataActionParamErrorContractDependenceIsPrecise(t *testing.T) {
	dependent := func(params map[string]string, format OutputFormat) DataActionParamError {
		t.Helper()
		_, err := NormalizeDataActionForOutputContract(DataAction{Kind: DataActionAssembleAnswer, Params: params}, OutputContract{Format: format})
		var paramErr DataActionParamError
		if !errors.As(err, &paramErr) {
			t.Fatalf("params=%v format=%s: err=%T %v, want DataActionParamError", params, format, err, err)
		}
		return paramErr
	}
	if got := dependent(map[string]string{"output_field": "count"}, OutputPlainSingleLine); !got.OutputContractDependent() || got.OutputFormat != OutputPlainSingleLine || got.Param != "output_field/projection" {
		t.Fatalf("omitted projection + output_field under plain: %+v, want contract-dependent rejection", got)
	}
	if got := dependent(map[string]string{"output_field": "count", "projection": "values"}, OutputJSONOnly); got.OutputContractDependent() {
		t.Fatalf("explicit values projection + output_field under json_only: %+v, must be contract-independent", got)
	}
	if got := dependent(map[string]string{"projection": "values", "bogus": "1"}, OutputPlainSingleLine); got.OutputContractDependent() {
		t.Fatalf("unknown parameter: %+v, must be contract-independent", got)
	}
	if got := dependent(map[string]string{"projection": "json_object"}, OutputCSVLine); !got.OutputContractDependent() || got.OutputFormat != OutputCSVLine {
		t.Fatalf("json_object under csv_line: %+v, want contract-dependent rejection naming csv_line", got)
	}
}
