package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const businessAnnotationTrace = `# tracer: nop
main-10 (10) [000] .... 1.010000: tracing_mark_write: B|10|NativeWork
main-10 (10) [000] .... 1.020000: tracing_mark_write: E|10
`

func businessAnnotationPublicFixture(t *testing.T) (*types.BusContext, string, types.ToolResult) {
	t.Helper()
	ctx, path := businessRefTestContext(t, businessAnnotationTrace)
	result := businessAnnotationQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": 1, "time_end": 1.03})
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, path, result
}

func businessAnnotationQuery(t *testing.T, ctx *types.BusContext, args map[string]any) types.ToolResult {
	t.Helper()
	raw, _ := json.Marshal(args)
	result, err := (&TraceQuery{skipNativeMeasurementMemo: true}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("business query: %v %s", err, result.Summary)
	}
	return result
}

func businessAnnotationCloneResult(result types.ToolResult) types.ToolResult {
	out := result // Keep only the result's genuinely private source receipt.
	raw, _ := json.Marshal(result.Observations)
	out.Observations = nil
	_ = json.Unmarshal(raw, &out.Observations)
	return out
}

func businessAnnotationMutateCarrier(record *types.ObservationRecord, mutate func(*types.RuntimeBusinessDiagramAnnotationPublication)) {
	for i, note := range record.RichNotes {
		if raw, ok := strings.CutPrefix(note, types.TraceNoteKeyBusinessDiagramAnnotation+"="); ok {
			var p types.RuntimeBusinessDiagramAnnotationPublication
			_ = json.Unmarshal([]byte(raw), &p)
			mutate(&p)
			changed, _ := json.Marshal(p)
			record.RichNotes[i] = types.TraceNoteKeyBusinessDiagramAnnotation + "=" + string(changed)
		}
	}
}

func businessAnnotationContract(ctx *types.BusContext, results ...types.ToolResult) *types.RuntimeMeasurementContract {
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(ctx, 0))
}

func TestHMC231BusinessAnnotationPublicCurrentContract(t *testing.T) {
	ctx, path, result := businessAnnotationPublicFixture(t)
	contract := businessAnnotationContract(ctx, result)
	annotations := contract.NativeBusinessDiagramAnnotations()
	if len(annotations) != 1 {
		t.Fatalf("public query did not qualify complete native pair: annotations=%+v choices=%+v", annotations, contract.Choices())
	}
	a := annotations[0]
	if a.Name != "NativeWork" || a.Owner != (types.RuntimeWakeupThread{PID: 10, TGID: 10, Comm: "main"}) ||
		a.StartTs != 1.01 || a.EndTs != 1.02 || a.StartLine != 2 || a.EndLine != 3 ||
		a.StartRef != path+":2" || a.EndRef != path+":3" || a.Selector == "" {
		t.Fatalf("native owner/physical interval changed: %+v", a)
	}
	cloned := contract.Clone()
	annotations[0].Record.RichNotes[0] = "caller mutation"
	annotations[0].Record.SupportRefs[0] = "other:999"
	if !reflect.DeepEqual(cloned.NativeBusinessDiagramAnnotations(), contract.NativeBusinessDiagramAnnotations()) ||
		contract.NativeBusinessDiagramAnnotations()[0].Record.RichNotes[0] == "caller mutation" {
		t.Fatal("caller mutated private annotation pool")
	}
	raw, _ := json.Marshal(contract)
	if strings.Contains(string(raw), a.Selector) {
		t.Fatal("private annotation pool leaked into contract JSON")
	}
	var restored types.RuntimeMeasurementContract
	if err := json.Unmarshal(raw, &restored); err != nil || len(restored.NativeBusinessDiagramAnnotations()) != 0 {
		t.Fatal("contract JSON restored annotation authority")
	}
	for _, table := range contract.Choices() {
		if table.MemberSet != nil || table.DisplayPopulation != nil || table.HasCompleteNativeDisplayPopulation() {
			t.Fatal("business annotation gained population/completion authority")
		}
	}
}

func TestHMC231BusinessAnnotationPublicExactNegativeBoundaries(t *testing.T) {
	ctx, _, result := businessAnnotationPublicFixture(t)
	if len(businessAnnotationContract(ctx, result).NativeBusinessDiagramAnnotations()) != 1 {
		t.Fatal("invalid positive fixture")
	}
	for _, mode := range []string{"source", "source_clock", "query_target", "query_scope", "window", "observation", "subject", "name", "owner", "owner_tgid", "time", "line", "start_ref", "end_ref", "support_ref", "duplicate_note", "duplicate_key", "unknown_field", "negative", "memo", "json_result", "no_summary", "conflicting_summary", "conflicting_carrier"} {
		t.Run(mode, func(t *testing.T) {
			copy := businessAnnotationCloneResult(result)
			index := -1
			for i, r := range copy.Observations {
				if r.Predicate == types.TraceBusinessSpanPredicate {
					index = i
					break
				}
			}
			if index < 0 {
				t.Fatal("no native business record")
			}
			r := &copy.Observations[index]
			businessAnnotationMutateCarrier(r, func(p *types.RuntimeBusinessDiagramAnnotationPublication) {
				switch mode {
				case "source":
					p.Source.Path += ".other"
				case "source_clock":
					p.Source.TimeDomain = "other-clock"
				case "query_target":
					p.Source.QueryTargetPID++
				case "query_scope":
					p.Source.QueryScopeID += ":other-view"
				case "window":
					p.Source.QueryWindowEndTs += 1
				case "observation":
					p.ObservationID += ":other"
				case "subject":
					p.Subject += "-other"
				case "name":
					p.Name += "-other"
				case "owner":
					p.Owner.PID++
				case "owner_tgid":
					p.Owner.TGID++
				case "time":
					p.Span.EndTs += .001
				case "line":
					p.Span.LineEnd++
				case "start_ref":
					p.StartRef += "0"
				case "end_ref":
					p.EndRef += "0"
				}
			})
			switch mode {
			case "support_ref":
				r.SupportRefs[0] += "0"
			case "negative":
				r.Negative = true
			case "memo":
				copy.ReusedFromRunMemo = true
			case "json_result":
				raw, _ := json.Marshal(copy)
				copy = types.ToolResult{}
				_ = json.Unmarshal(raw, &copy)
			case "duplicate_note", "duplicate_key", "unknown_field", "no_summary":
				var notes []string
				for _, note := range r.RichNotes {
					if mode == "no_summary" && strings.HasPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"=") {
						continue
					}
					if strings.HasPrefix(note, types.TraceNoteKeyBusinessDiagramAnnotation+"=") {
						switch mode {
						case "duplicate_note":
							notes = append(notes, note)
						case "duplicate_key":
							note = strings.Replace(note, "{", `{"version":1,`, 1)
						case "unknown_field":
							note = strings.Replace(note, "{", `{"extra":true,`, 1)
						}
					}
					notes = append(notes, note)
				}
				r.RichNotes = notes
			case "conflicting_carrier":
				duplicate := businessAnnotationCloneResult(copy).Observations[index]
				businessAnnotationMutateCarrier(&duplicate, func(p *types.RuntimeBusinessDiagramAnnotationPublication) { p.Owner.TGID++ })
				copy.Observations = append(copy.Observations, duplicate)
			case "conflicting_summary":
				duplicate := businessAnnotationCloneResult(copy).Observations[index]
				for i, note := range duplicate.RichNotes {
					if raw, ok := strings.CutPrefix(note, types.TraceNoteKeyRuntimeMeasurement+"="); ok {
						var p types.RuntimeMeasurementPublication
						_ = json.Unmarshal([]byte(raw), &p)
						p.Tables[0].Rows[0][1] += " altered"
						data, _ := json.Marshal(p)
						duplicate.RichNotes[i] = types.TraceNoteKeyRuntimeMeasurement + "=" + string(data)
					}
				}
				copy.Observations = append(copy.Observations, duplicate)
			}
			if got := businessAnnotationContract(ctx, copy).NativeBusinessDiagramAnnotations(); len(got) != 0 {
				t.Fatalf("%s gained current native annotation: %+v", mode, got)
			}
		})
	}
	if got := businessAnnotationContract(ctx, result, result).NativeBusinessDiagramAnnotations(); len(got) != 1 {
		t.Fatal("identical native publication repeated its annotation")
	}
}

func TestHMC231BusinessAnnotationPublicCompleteEndpointsAndGeneration(t *testing.T) {
	ctx, path, result := businessAnnotationPublicFixture(t)
	initial := businessAnnotationContract(ctx, result).NativeBusinessDiagramAnnotations()
	if len(initial) != 1 {
		t.Fatal("no initial annotation")
	}
	for _, window := range [][2]float64{{1.015, 1.03}, {1, 1.015}, {1, 1.02}} {
		out := businessAnnotationQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": window[0], "time_end": window[1]})
		if len(businessAnnotationContract(ctx, out).NativeBusinessDiagramAnnotations()) != 0 {
			t.Fatalf("clipped or excluded endpoint gained annotation: %v", window)
		}
	}
	// Equal bytes at the same pathname are a different physical generation.
	replacement := filepath.Join(filepath.Dir(path), "replacement.trace")
	if err := os.WriteFile(replacement, []byte(businessAnnotationTrace), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if len(businessAnnotationContract(ctx, result).NativeBusinessDiagramAnnotations()) != 0 {
		t.Fatal("stale source generation retained annotation authority")
	}
	params, _ := json.Marshal(map[string]any{"path": path, "view": "window_stats", "time_start": 1, "time_end": 1.03})
	fresh, err := (&TraceQuery{skipNativeMeasurementMemo: true}).Execute(ctx, params)
	if err != nil || !fresh.Success {
		t.Fatalf("fresh query: %v %s", err, fresh.Summary)
	}
	current := businessAnnotationContract(ctx, fresh).NativeBusinessDiagramAnnotations()
	if len(current) != 1 || current[0].Selector == initial[0].Selector {
		t.Fatal("fresh source did not receive a distinct current selector")
	}
}

func TestHMC231BusinessAnnotationPublicLongOwnerLabelCollision(t *testing.T) {
	comm := strings.Repeat("x", 220)
	ctx, path := businessRefTestContext(t, strings.ReplaceAll(businessAnnotationTrace, "main-", comm+"-"))
	result := businessAnnotationQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": 1, "time_end": 1.03})
	initial := businessAnnotationContract(ctx, result).NativeBusinessDiagramAnnotations()
	if len(initial) != 1 || initial[0].Owner.Comm != comm {
		t.Fatalf("original untruncated typed owner was lost: annotations=%+v", initial)
	}
	changed := businessAnnotationCloneResult(result)
	for i := range changed.Observations {
		if changed.Observations[i].Predicate != types.TraceBusinessSpanPredicate {
			continue
		}
		businessAnnotationMutateCarrier(&changed.Observations[i], func(p *types.RuntimeBusinessDiagramAnnotationPublication) {
			p.Owner.Comm = p.Owner.Comm[:len(p.Owner.Comm)-1] + "y"
			if types.RuntimeWakeupThreadLabel(p.Owner) != changed.Observations[i].Subject {
				t.Fatal("fixture did not retain the same truncated reader label")
			}
		})
	}
	if len(businessAnnotationContract(ctx, changed).NativeBusinessDiagramAnnotations()) != 0 {
		t.Fatal("a colliding reader label lent a different typed owner annotation authority")
	}
}
