package dataquery

import "strings"

// ActionOutputProjectionContract is the executor-owned vocabulary for one
// explicit assemble_answer projection. Canonical keeps compatibility aliases
// on a single runtime meaning. Formats is empty for text projections whose
// exact shape is checked by ValidateAnswer; non-empty means the projection
// owns an encoding that is incompatible with every other strict format.
type ActionOutputProjectionContract struct {
	Value     string
	Canonical string
	Formats   []OutputFormat
}

var actionOutputProjectionContracts = []ActionOutputProjectionContract{
	{Value: "values", Canonical: "values", Formats: []OutputFormat{OutputPlainSingleLine, OutputCSVLine, OutputMarkdown, OutputFilePath, OutputFreeform}},
	{Value: "value_list", Canonical: "values", Formats: []OutputFormat{OutputPlainSingleLine, OutputCSVLine, OutputMarkdown, OutputFilePath, OutputFreeform}},
	{Value: "csv_values", Canonical: "values", Formats: []OutputFormat{OutputPlainSingleLine, OutputCSVLine, OutputMarkdown, OutputFilePath, OutputFreeform}},
	{Value: "key_values", Canonical: "key_values", Formats: []OutputFormat{OutputPlainSingleLine, OutputCSVLine, OutputMarkdown, OutputFreeform}},
	{Value: "json_groups", Canonical: "json_groups", Formats: []OutputFormat{OutputJSONOnly, OutputFreeform}},
	{Value: "json", Canonical: "json_groups", Formats: []OutputFormat{OutputJSONOnly, OutputFreeform}},
	{Value: "json_object", Canonical: "json_object", Formats: []OutputFormat{OutputJSONOnly, OutputFreeform}},
	{Value: "json_object_values", Canonical: "json_object", Formats: []OutputFormat{OutputJSONOnly, OutputFreeform}},
	{Value: "object", Canonical: "json_object", Formats: []OutputFormat{OutputJSONOnly, OutputFreeform}},
	{Value: "markdown_table", Canonical: "markdown_table", Formats: []OutputFormat{OutputMarkdownTable, OutputMarkdown, OutputFreeform}},
}

var actionOutputProjectionFormats = []OutputFormat{
	OutputPlainSingleLine,
	OutputCSVLine,
	OutputJSONOnly,
	OutputMarkdownTable,
	OutputMarkdown,
	OutputFilePath,
	OutputFreeform,
}

// DataActionOutputProjectionContracts returns defensive copies for planner
// schema projection. Runtime admission and model teaching therefore share the
// same explicit vocabulary and strict-format compatibility rules.
func DataActionOutputProjectionContracts() []ActionOutputProjectionContract {
	out := make([]ActionOutputProjectionContract, 0, len(actionOutputProjectionContracts))
	for _, contract := range actionOutputProjectionContracts {
		copy := contract
		copy.Formats = append([]OutputFormat(nil), contract.Formats...)
		out = append(out, copy)
	}
	return out
}

// DataActionOutputProjectionFormats enumerates the output-contract formats
// for which the planner can project the runtime compatibility matrix.
func DataActionOutputProjectionFormats() []OutputFormat {
	return append([]OutputFormat(nil), actionOutputProjectionFormats...)
}

// DataActionOutputProjectionsForFormat returns every explicit projection the
// executor admits for one typed output format, including compatibility names.
func DataActionOutputProjectionsForFormat(format OutputFormat) []string {
	format = OutputContract{Format: format}.Normalize().Format
	values := make([]string, 0, len(actionOutputProjectionContracts))
	for _, contract := range actionOutputProjectionContracts {
		if len(contract.Formats) == 0 || outputFormatIn(format, contract.Formats) {
			values = append(values, contract.Value)
		}
	}
	return values
}

func normalizeActionOutputProjection(raw string, format OutputFormat) (string, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return "", nil
	}
	format = OutputContract{Format: format}.Normalize().Format
	for _, contract := range actionOutputProjectionContracts {
		if raw != contract.Value {
			continue
		}
		if len(contract.Formats) == 0 || outputFormatIn(format, contract.Formats) {
			return contract.Canonical, nil
		}
		return "", DataActionParamError{
			ActionKind:    DataActionAssembleAnswer,
			Param:         "projection/output_contract.format",
			ExpectedShape: "a projection whose owned encoding matches output_contract.format",
			ActualSnippet: "projection=" + raw + ", format=" + string(format),
			Message:       "assemble_answer projection=" + raw + " owns a different output encoding and cannot satisfy output_contract.format=" + string(format) + "; choose a projection for the declared format instead of publishing a structurally complete answer with the wrong shape",
		}
	}
	return "", DataActionParamError{
		ActionKind:    DataActionAssembleAnswer,
		Param:         "projection",
		ExpectedShape: "one executor-supported assemble_answer projection",
		ActualSnippet: raw,
		Message:       "assemble_answer projection is not recognized by the executor; use the typed projection enum or omit projection so the output contract selects the default",
	}
}

func outputFormatIn(format OutputFormat, formats []OutputFormat) bool {
	for _, candidate := range formats {
		if format == candidate {
			return true
		}
	}
	return false
}
