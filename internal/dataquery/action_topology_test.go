package dataquery

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// V9-1 (§40.15) structural pin: the runner of every 1:N action stamps a
// derived row identity through the single helper, and no 1:1 runner does
// (1:1 identity is inherited, never re-minted). The kind→runner map is read
// off the ActionRunner.Run dispatch switch so a new 1:N kind whose runner
// forgets the stamp goes red here, not in a customer ledger.
func TestOneToManyRunnersStampDerivedRowIdentity(t *testing.T) {
	data, err := os.ReadFile("action_runner.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	// Each `case DataActionX:` arm of the Run dispatch switch calls exactly
	// one r.run<Y>(action, ...) runner; map the kind to that runner.
	runCall := regexp.MustCompile(`r\.(run[A-Za-z]+)\(action`)
	runners := map[string]string{}
	arms := strings.Split(src, "\n\t\tcase DataAction")
	for _, arm := range arms[1:] {
		colon := strings.Index(arm, ":")
		if colon < 0 {
			continue
		}
		kindName := "DataAction" + arm[:colon]
		if match := runCall.FindStringSubmatch(arm); match != nil {
			if _, dup := runners[kindName]; !dup {
				runners[kindName] = match[1]
			}
		}
	}
	for _, kindName := range []string{"DataActionExpandRecords", "DataActionJoinRecords", "DataActionFilterRecords", "DataActionQualifyRecords", "DataActionGroupRecords"} {
		if runners[kindName] == "" {
			t.Fatalf("dispatch switch in ActionRunner.Run does not route %s through a run<X>(action, ...) runner; extend the pin's dispatch pattern", kindName)
		}
	}
	kindByName := map[string]DataActionKind{
		"DataActionExpandRecords":    DataActionExpandRecords,
		"DataActionJoinRecords":      DataActionJoinRecords,
		"DataActionGroupRecords":     DataActionGroupRecords,
		"DataActionFilterRecords":    DataActionFilterRecords,
		"DataActionQualifyRecords":   DataActionQualifyRecords,
		"DataActionEnrichRecords":    DataActionEnrichRecords,
		"DataActionApplyResolutions": DataActionApplyResolutions,
		"DataActionDeriveFields":     DataActionDeriveFields,
		"DataActionExtractFields":    DataActionExtractFields,
	}
	for kindName, runner := range runners {
		kind, known := kindByName[kindName]
		if !known {
			continue
		}
		topology, ok := DataActionTopologyOf(kind)
		if !ok {
			t.Fatalf("%s has a runner but no declared topology", kindName)
		}
		body := runnerBody(src, runner)
		if body == "" {
			t.Fatalf("runner %s for %s not found", runner, kindName)
		}
		stamps := strings.Contains(body, "stampDerivedRowIdentity(")
		switch topology {
		case ActionTopologyOneToMany:
			if !stamps {
				t.Errorf("%s is 1:N but %s never calls stampDerivedRowIdentity (siblings would share identity)", kindName, runner)
			}
		case ActionTopologyOneToOne:
			if stamps {
				t.Errorf("%s is 1:1 but %s re-mints identity via stampDerivedRowIdentity (identity must be inherited)", kindName, runner)
			}
		case ActionTopologyManyToOne:
			if !strings.Contains(body, "stampGroupRowIdentity(") {
				t.Errorf("%s is N:1 but %s does not mint an artifact-local group identity via stampGroupRowIdentity", kindName, runner)
			}
		}
	}
}

func runnerBody(src, runner string) string {
	start := strings.Index(src, "func (r ActionRunner) "+runner+"(")
	if start < 0 {
		return ""
	}
	rest := src[start:]
	end := strings.Index(rest[1:], "\nfunc ")
	if end < 0 {
		return rest
	}
	return rest[:end+1]
}

// Every action kind constant declares a topology (closed set), and the table
// names no kind the runner does not know.
func TestDataActionTopologyCoversEveryKindConstant(t *testing.T) {
	data, err := os.ReadFile("dataquery.go")
	if err != nil {
		t.Fatal(err)
	}
	constRE := regexp.MustCompile(`DataAction[A-Za-z]+\s+DataActionKind = "([a-z_]+)"`)
	declared := map[string]bool{}
	for _, match := range constRE.FindAllStringSubmatch(string(data), -1) {
		declared[match[1]] = true
		topology, ok := DataActionTopologyOf(DataActionKind(match[1]))
		if !ok {
			t.Errorf("action kind %q declares no derivation topology (add it to dataActionTopologies)", match[1])
			continue
		}
		switch topology {
		case ActionTopologyOneToOne, ActionTopologyOneToMany, ActionTopologyManyToOne, ActionTopologyNone:
		default:
			t.Errorf("action kind %q declares topology %q outside the closed set", match[1], topology)
		}
	}
	if len(declared) == 0 {
		t.Fatal("no DataActionKind constants found; the pin's pattern drifted")
	}
	for _, kind := range DeclaredTopologyActionKinds() {
		if !declared[string(kind)] {
			t.Errorf("dataActionTopologies names %q, which is not a DataActionKind constant", kind)
		}
	}
	if _, ok := DataActionTopologyOf(" Expand_Records "); !ok {
		t.Fatal("topology lookup must normalize kind spelling like the runner does")
	}
	if _, ok := DataActionTopologyOf("no_such_action"); ok {
		t.Fatal("unknown kinds must not resolve a topology")
	}
}
