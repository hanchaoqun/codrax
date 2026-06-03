package operation

import (
	"fmt"
	"strings"
)

// Request is the typed operation intent that the REPL classifier produced.
// It intentionally carries policy fields rather than raw user prose heuristics:
// operation routing must stay driven by the classifier's structured signals.
type Request struct {
	Text                 string
	Operation            string
	OperationKind        string
	Source               string
	NeedsRepoAccess      bool
	RiskLevel            string
	SideEffects          []string
	TargetSurface        string
	RequiresConfirmation bool
}

// ProviderInfo describes a future executable operation provider. Batch 2 keeps
// the provider list empty in production; the shape is here so MCP/desktop/office
// integrations can plug in later without changing REPL policy semantics.
type ProviderInfo struct {
	Name         string
	Kind         string
	Surfaces     []string
	SideEffects  []string
	RequiresGate bool
}

// Step is one deterministic operation-plan row.
type Step struct {
	Title   string
	Detail  string
	Blocked bool
}

// Plan is the user-visible operation preflight. It is advisory and read-only:
// CanExecute=false means Codrax must not call side-effecting tools.
type Plan struct {
	Kind                 string
	Source               string
	NeedsRepoAccess      bool
	RiskLevel            string
	SideEffects          []string
	TargetSurface        string
	RequiresConfirmation bool
	CanExecute           bool
	Provider             string
	MissingCapability    string
	Steps                []Step
}

// BuildPlan converts a typed operation request into a bounded plan. It does no
// IO and does not inspect the repository. That separation is deliberate: code
// analysis and external artifact/desktop operations have different evidence and
// side-effect contracts.
func BuildPlan(req Request, providers []ProviderInfo) Plan {
	kind := firstNonEmpty(req.OperationKind, req.Operation, "computer_operation")
	risk := firstNonEmpty(req.RiskLevel, "low")
	target := firstNonEmpty(req.TargetSurface, defaultSurface(kind))
	sideEffects := cleanList(req.SideEffects)
	requiresConfirmation := req.RequiresConfirmation || strings.EqualFold(risk, "high")

	provider := matchProvider(kind, target, providers)
	canExecute := provider.Name != "" && !requiresConfirmation
	missing := ""
	if provider.Name == "" {
		missing = fmt.Sprintf("no configured operation provider for %s on %s", kind, target)
	} else if requiresConfirmation {
		missing = "operation requires explicit confirmation before execution"
	}

	steps := []Step{}
	if req.NeedsRepoAccess {
		steps = append(steps, Step{
			Title:   "gather source facts",
			Detail:  "collect repository facts in a source-analysis lane before producing the artifact",
			Blocked: true,
		})
	}
	steps = append(steps, kindSteps(kind, target)...)
	if missing != "" {
		steps = append(steps, Step{
			Title:   "execution paused",
			Detail:  missing,
			Blocked: true,
		})
	}

	return Plan{
		Kind:                 kind,
		Source:               firstNonEmpty(req.Source, "current_message"),
		NeedsRepoAccess:      req.NeedsRepoAccess,
		RiskLevel:            risk,
		SideEffects:          sideEffects,
		TargetSurface:        target,
		RequiresConfirmation: requiresConfirmation,
		CanExecute:           canExecute,
		Provider:             provider.Name,
		MissingCapability:    missing,
		Steps:                steps,
	}
}

func kindSteps(kind, target string) []Step {
	switch kind {
	case "presentation_generation":
		return []Step{
			{Title: "outline deck", Detail: "turn the requested material into slide sections and speaker-facing structure"},
			{Title: "create slides", Detail: "produce a local presentation artifact and verify the rendered layout"},
		}
	case "document_generation":
		return []Step{
			{Title: "outline document", Detail: "turn the requested material into sections and claims"},
			{Title: "create document", Detail: "produce a local document artifact and verify the rendered pages"},
		}
	case "spreadsheet_generation":
		return []Step{
			{Title: "shape workbook", Detail: "identify sheets, tables, formulas, and charts"},
			{Title: "create spreadsheet", Detail: "produce a local workbook and verify recalculation/rendering"},
		}
	case "browser_operation":
		return []Step{
			{Title: "open browser workflow", Detail: "operate the browser surface with visible, cancellable steps"},
			{Title: "verify result", Detail: "inspect the browser state before reporting completion"},
		}
	case "external_skill_workflow":
		return []Step{
			{Title: "select external workflow", Detail: "choose the configured external skill/MCP workflow by typed capability"},
			{Title: "run workflow", Detail: "execute through the provider's bounded interface and return artifacts/observations"},
		}
	default:
		return []Step{
			{Title: "plan operation", Detail: fmt.Sprintf("prepare a bounded %s workflow for target surface %s", kind, target)},
			{Title: "verify result", Detail: "inspect generated artifacts or UI state before reporting completion"},
		}
	}
}

func matchProvider(kind, target string, providers []ProviderInfo) ProviderInfo {
	for _, p := range providers {
		if strings.TrimSpace(p.Name) == "" {
			continue
		}
		if !matchesOne(kind, p.Kind) {
			continue
		}
		if target != "" && target != "unknown" && len(p.Surfaces) > 0 && !contains(p.Surfaces, target) {
			continue
		}
		return p
	}
	return ProviderInfo{}
}

func defaultSurface(kind string) string {
	switch kind {
	case "presentation_generation":
		return "slides"
	case "document_generation":
		return "office_doc"
	case "spreadsheet_generation":
		return "spreadsheet"
	case "browser_operation":
		return "browser"
	case "computer_operation":
		return "desktop"
	default:
		return "unknown"
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func matchesOne(value, pattern string) bool {
	value = strings.TrimSpace(value)
	pattern = strings.TrimSpace(pattern)
	return pattern == "" || pattern == "*" || value == pattern
}

func contains(values []string, needle string) bool {
	for _, v := range values {
		if strings.TrimSpace(v) == needle {
			return true
		}
	}
	return false
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
