package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const concreteValueSourceExpression = "source_expression"

// The cache and the read adapter consume this same precise coordinate. Outputs
// of this enrichment cannot become fresh reading authority for themselves.
func concreteExactEvidenceCoordinate(item types.EvidenceItem) (string, int, bool) {
	if types.EvidenceIsDerivationCandidate(item) || !item.IsCitable() || item.LineStart <= 0 || (item.LineEnd != 0 && item.LineEnd != item.LineStart) {
		return "", 0, false
	}
	switch item.Producer {
	case "concrete_values", "bridge_literal", "bridge_literal_terminal",
		types.EvidenceProducerRepoMapDynamicSelectorAssignment,
		types.EvidenceProducerRepoMapDynamicSelectorReturn,
		types.EvidenceProducerRepoMapDynamicSelectorArgument:
		return "", 0, false
	}
	source := canonicalExplorerPath(item.Source)
	return source, item.LineStart, source != ""
}

func concreteExactEvidenceCoverageKey(evidence []types.EvidenceItem) string {
	coordinates := make(map[string]bool)
	for _, item := range evidence {
		if file, line, ok := concreteExactEvidenceCoordinate(item); ok {
			coordinates[fmt.Sprintf("%s\x00%d", file, line)] = true
		}
	}
	rows := make([]string, 0, len(coordinates))
	for coordinate := range coordinates {
		rows = append(rows, coordinate)
	}
	sort.Strings(rows)
	encoded, _ := json.Marshal(rows)
	return "\x00exact-evidence-coordinates=" + string(encoded)
}

// Only the parser adapter assigns an expression endLine. Preserve its bounded
// source expression even when formatted over multiple lines; the old lexical
// prose filter and all chain-inference filters retain their original behavior.
func concreteValueIsBoundedSourceFact(value concreteValue) bool {
	if value.kind == concreteValueKindReturns && value.endLine >= value.line && value.endLine > 0 && !value.candidate {
		return len(value.value) <= proseConcreteValueMaxLen
	}
	return !isProseLikeConcreteValue(value.value)
}

type concreteReturnSourceSnapshots map[string][]byte

func (s concreteReturnSourceSnapshots) read(repoRoot, file string) []byte {
	if source, ok := s[file]; ok {
		return source
	}
	source, _ := os.ReadFile(filepath.Join(repoRoot, file))
	if s != nil {
		s[file] = source // nil is a recorded failed read, not the previous source.
	}
	return source
}

func refreshedConcreteReturnSources(repoRoot string, previous concreteReturnSourceSnapshots) concreteReturnSourceSnapshots {
	current := make(concreteReturnSourceSnapshots, len(previous))
	for file := range previous {
		current.read(repoRoot, file)
	}
	return current
}

// Cache identity includes the parser's semantic inputs and the exact source
// snapshots that were actually inspected. A changed/failed read or removed
// receipt cannot keep a warm independently-proved return alive. This key does
// not mark files read and never reads a new path merely because it has receipts.
func concreteReturnInputGenerationKey(graph *repotypes.Graph, sources concreteReturnSourceSnapshots) string {
	h := sha256.New()
	if graph != nil {
		files := make([]string, 0, len(graph.FileIndex))
		for file := range graph.FileIndex {
			files = append(files, file)
		}
		sort.Strings(files)
		for _, file := range files {
			fi := graph.FileIndex[file]
			if fi == nil {
				continue
			}
			type owner struct {
				Name, Receiver, Parent, File, Kind string
				Line, EndLine, BodyStart, BodyEnd  int
				Presence                           repotypes.CallableBodyPresence
			}
			owners := make([]owner, 0, len(fi.Symbols))
			for _, sym := range fi.Symbols {
				owners = append(owners, owner{sym.Name, sym.Receiver, sym.Parent, sym.File, sym.Kind, sym.Line, sym.EndLine, sym.BodyStartLine, sym.BodyEndLine, sym.BodyPresence})
			}
			_ = json.NewEncoder(h).Encode(struct {
				File, Path, Hash string
				Owners           []owner
				Returns          []repotypes.CallableReturnExpression
			}{file, fi.RelPath, fi.Hash, owners, fi.CallableReturnExpressions})
		}
	}
	files := make([]string, 0, len(sources))
	for file := range sources {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		source := sources[file]
		_ = json.NewEncoder(h).Encode(struct {
			File    string
			Present bool
			Hash    [32]byte
		}{file, source != nil, sha256.Sum256(source)})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// concreteCallableReturnValues consumes the parser's exact callable/return
// receipt. FileInfo verifies the complete source snapshot and expression bytes;
// this adapter only adds the existing read/evidence occurrence boundary. No
// owner name, line feature, nearby return, or expression spelling proves a
// return on its own.
func concreteCallableReturnValues(fi *repotypes.FileInfo, sym repotypes.Symbol, reader *repotypes.CallableReturnReader, readSet map[string]bool, closure *types.EvidenceClosure, evidence []types.EvidenceItem) []concreteValue {
	var values []concreteValue
	for _, value := range concreteParserCallableReturnValues(fi, sym, reader) {
		allowed := true
		for line := value.line; line <= value.endLine; line++ {
			if !runtimeTargetReadOrExactEvidenceLineAllowed(fi.RelPath, line, readSet, closure, evidence) {
				allowed = false
				break
			}
		}
		if !allowed {
			continue
		}
		values = append(values, value)
	}
	return values
}

// The bridge-literal lane already owns a separate source/read admission policy;
// it shares this parser projection without inventing a read-set receipt.
func concreteParserCallableReturnValues(fi *repotypes.FileInfo, sym repotypes.Symbol, reader *repotypes.CallableReturnReader) []concreteValue {
	if fi == nil || reader == nil {
		return nil
	}
	var values []concreteValue
	for _, receipt := range reader.For(sym) {
		owner := sym.Receiver
		if owner == "" {
			owner = sym.Parent
		}
		method := sym.Name
		if owner != "" {
			method = owner + "." + sym.Name
		}
		values = append(values, concreteValue{
			file: fi.RelPath, receiver: owner, method: method, kind: concreteValueKindReturns,
			value: receipt.Expression, line: receipt.LineStart, endLine: receipt.LineEnd,
		})
	}
	return values
}

// concreteUnprovedReturnLead retains a syntactic candidate as its original
// source line, without a return anchor or a claim about the enclosing callable.
// Exact parser returns are published separately, so a lexical scanner cannot
// grant a nested closure's result to its parent or turn a branch assignment
// into a returned value.
func concreteUnprovedReturnLead(entry concreteValueEntry, source string, snippetStart int, proven []concreteValue) (concreteValueEntry, bool) {
	if entry.kind != concreteValueKindReturns {
		return entry, true
	}
	line := concreteValueAbsoluteLine(snippetStart, entry.lineOffset)
	for _, value := range proven {
		if value.line == line && value.value == entry.value {
			return concreteValueEntry{}, false
		}
	}
	lines := strings.Split(source, "\n")
	if entry.lineOffset < 0 || entry.lineOffset >= len(lines) {
		return concreteValueEntry{}, false
	}
	entry.kind = concreteValueSourceExpression
	entry.value = strings.TrimSpace(lines[entry.lineOffset])
	entry.candidate = true
	return entry, entry.value != ""
}

func concreteValueLastLine(value concreteValue) int {
	if value.endLine >= value.line {
		return value.endLine
	}
	return value.line
}
