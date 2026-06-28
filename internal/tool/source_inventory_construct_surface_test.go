package tool

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceInventoryConstructSurfaceTerms_PublicModifiedTypeCarriesBaseFamily(t *testing.T) {
	terms := sourceInventoryConstructSurfaceTerms(&repotypes.Symbol{
		Name:     "Animal",
		Kind:     "class",
		Exported: true,
		Doc:      "public sealed",
	})
	joined := strings.Join(terms, "\n")
	for _, want := range []string{
		"public class",
		"public class Animal",
		"public sealed class",
		"public sealed class Animal",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("surface terms missing %q: %#v", want, terms)
		}
	}
	if len(terms) < 2 || terms[0] != "public class" || terms[1] != "public class Animal" {
		t.Fatalf("base public class family should lead exact modifiers for grouping; got %#v", terms)
	}
}

func TestSourceInventoryConstructSurfaceTerms_OpenTypeDoesNotBecomePublicFamily(t *testing.T) {
	terms := sourceInventoryConstructSurfaceTerms(&repotypes.Symbol{
		Name:     "Hook",
		Kind:     "class",
		Exported: true,
		Doc:      "open",
	})
	joined := strings.Join(terms, "\n")
	if strings.Contains(joined, "public class") {
		t.Fatalf("open-only class must not be classified as public class: %#v", terms)
	}
	if !strings.Contains(joined, "open class") || !strings.Contains(joined, "open class Hook") {
		t.Fatalf("open class surface terms missing: %#v", terms)
	}
}

func TestSourceInventorySurfaceFamilyGroups_PublicClassModifierVariants(t *testing.T) {
	rows := []types.SourceInventoryRow{
		sourceInventorySurfaceFamilyTestRow("Dog", "dog.cj", 10, []string{"public class", "public class Dog"}),
		sourceInventorySurfaceFamilyTestRow("Animal", "animal.cj", 6, []string{"public class", "public class Animal", "public sealed class", "public sealed class Animal"}),
		sourceInventorySurfaceFamilyTestRow("Service", "service.cj", 32, []string{"public class", "public class Service", "public abstract class", "public abstract class Service"}),
	}
	groups := sourceInventorySurfaceFamilyGroups(rows)
	if len(groups) != 1 {
		t.Fatalf("public class modifier variants should form one surface family, got %+v", groups)
	}
	if groups[0].family != "public class" || len(groups[0].members) != 3 {
		t.Fatalf("surface family = %+v, want public class with three members", groups[0])
	}
}

func sourceInventorySurfaceFamilyTestRow(name, file string, line int, terms []string) types.SourceInventoryRow {
	return types.SourceInventoryRow{
		Role: types.AnswerCandidateRoleType,
		Member: types.SourceInventoryObservationMember{
			Name:         name,
			Role:         types.AnswerCandidateRoleType,
			File:         file,
			Line:         line,
			SurfaceTerms: terms,
		},
	}
}
