package tracediag

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Row positions come from the typed renderer, never from matching report text.
// The native preview census and the report's actually written complete rows
// have separate authorities, including when the outer report cap is reached.
type packetReportAccounting struct {
	memberRows   []int
	categoryRows []int
	nativeStatus string
	failure      string
}

func (a *packetReportAccounting) summary(shownLines int) string {
	visible := func(rows []int) int {
		count := 0
		for _, row := range rows {
			if row < shownLines {
				count++
			}
		}
		return count
	}
	members, categories := visible(a.memberRows), visible(a.categoryRows)
	return fmt.Sprintf("native_status=%s native_preview_members=%d displayed_members=%d undisplayed_preview_members=%d native_classification_entries=%d displayed_classification_entries=%d undisplayed_classification_entries=%d%s; 展示省略不改变原生聚合或查询完成状态", a.nativeStatus, len(a.memberRows), members, len(a.memberRows)-members, len(a.categoryRows), categories, len(a.categoryRows)-categories, a.failure)
}

// Packet records and endpoint groups are bounded native preview members. Each
// member is one physical report line, retaining every existing exact DTO path
// and value. This keeps both line caps and their prefix contract while making
// a member indivisible at either cap. Compact classification/exclusion rows
// precede members; required scalars, header, clocks and failure stay first.
// Nothing here changes the native result, its preview size or its completion.
func renderPacketStepBody(step *Step, res *tracequery.Result) stepBody {
	sink := &bodySink{cap: step.EffectiveMaxLines()}
	a := &packetReportAccounting{}
	emit := func(line string) {
		// Native tokens may contain quoted text. No embedded newline can create
		// uncounted physical lines or split one admitted semantic member.
		sink.emit(strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(line))
	}
	renderResultMeta(res, emit)
	var root reflect.Value
	var categories []string
	if res.PacketInventory != nil && res.View == tracequery.ViewPacketInventory {
		root = reflect.ValueOf(res.PacketInventory).Elem()
		categories = []string{"Protocols", "DecodeStatuses", "Directions"}
	} else {
		root = reflect.ValueOf(res.PacketFlows).Elem()
		categories = []string{"Ungrouped"}
	}
	a.nativeStatus = formatScalar(root.FieldByName("Status"))
	if failure := root.FieldByName("Failure"); !failure.IsNil() {
		var lines []string
		renderExactNativeDTO(failure.Elem(), "failure", func(line string) { lines = append(lines, line) }, 0, nil)
		a.failure = " native_failure={" + strings.Join(lines, " | ") + "}"
	}
	path := res.View
	emit("明细 " + path + ":")
	// A shallow display copy keeps all scalar/optional fields and any future
	// additions visible through the reviewed exact DTO walker. Only these
	// existing collections receive a separate atomic display schedule.
	head := reflect.New(root.Type()).Elem()
	head.Set(root)
	for _, field := range append([]string{"Members"}, categories...) {
		value := head.FieldByName(field)
		value.Set(reflect.Zero(value.Type()))
	}
	renderExactNativeDTO(head, path, emit, 0, &nonEventDetailPolicy)
	emitCollection := func(name string, positions *[]int) {
		field, _ := root.Type().FieldByName(name)
		members := root.FieldByIndex(field.Index)
		for i := 0; i < members.Len(); i++ {
			var lines []string
			walkDetailWithPolicy(members.Index(i), fmt.Sprintf("%s.%s[%d]", path, jsonTagName(field), i), func(line string) { lines = append(lines, line) }, 1, &nonEventDetailPolicy)
			if len(lines) != 0 {
				*positions = append(*positions, sink.total)
				emit(strings.Join(lines, " | "))
			}
		}
	}
	for _, name := range categories {
		emitCollection(name, &a.categoryRows)
	}
	emitCollection("Members", &a.memberRows)
	return stepBody{lines: sink.lines, total: sink.total, packet: a}
}
