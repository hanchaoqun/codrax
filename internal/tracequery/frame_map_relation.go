package tracequery

import (
	"fmt"
	"strconv"
	"strings"
)

const frameMapRelationPrefix = "# codrax_frame_map/v1"

// FrameMapRelation is a converter-authored copy of one proven trace_streamer
// frame_maps row. It remains a comment for generic ftrace readers and becomes
// a typed relation in Codrax. TimestampNS is the destination frame start and
// SourceTimestampNS is retained separately; neither value is a duration.
type FrameMapRelation struct {
	TimestampNS       uint64
	RelationID        uint32
	SourceRow         uint32
	DestinationRow    uint32
	SourceTimestampNS uint64
}

func FormatFrameMapRelation(relation FrameMapRelation) (string, error) {
	if !validFrameMapRelation(relation) {
		return "", fmt.Errorf("invalid frame-map relation")
	}
	return strings.Join([]string{
		frameMapRelationPrefix,
		"ts_ns=" + strconv.FormatUint(relation.TimestampNS, 10),
		"relation_id=" + strconv.FormatUint(uint64(relation.RelationID), 10),
		"src_row=" + strconv.FormatUint(uint64(relation.SourceRow), 10),
		"dst_row=" + strconv.FormatUint(uint64(relation.DestinationRow), 10),
		"src_ts_ns=" + strconv.FormatUint(relation.SourceTimestampNS, 10),
	}, " "), nil
}

func parseFrameMapRelation(line string) (FrameMapRelation, bool) {
	if !strings.HasPrefix(line, frameMapRelationPrefix+" ") {
		return FrameMapRelation{}, false
	}
	parts := strings.Split(line, " ")
	if len(parts) != 7 || parts[0] != "#" || parts[1] != "codrax_frame_map/v1" {
		return FrameMapRelation{}, false
	}
	value := func(index int, prefix string) (string, bool) {
		if !strings.HasPrefix(parts[index], prefix) {
			return "", false
		}
		out := strings.TrimPrefix(parts[index], prefix)
		return out, out != ""
	}
	tsRaw, tsOK := value(2, "ts_ns=")
	relationRaw, relationOK := value(3, "relation_id=")
	sourceRaw, sourceOK := value(4, "src_row=")
	destinationRaw, destinationOK := value(5, "dst_row=")
	sourceTSRaw, sourceTSOK := value(6, "src_ts_ns=")
	if !tsOK || !relationOK || !sourceOK || !destinationOK || !sourceTSOK {
		return FrameMapRelation{}, false
	}
	timestamp, err := strconv.ParseUint(tsRaw, 10, 64)
	if err != nil {
		return FrameMapRelation{}, false
	}
	relationID, err := strconv.ParseUint(relationRaw, 10, 32)
	if err != nil {
		return FrameMapRelation{}, false
	}
	sourceRow, err := strconv.ParseUint(sourceRaw, 10, 32)
	if err != nil {
		return FrameMapRelation{}, false
	}
	destinationRow, err := strconv.ParseUint(destinationRaw, 10, 32)
	if err != nil {
		return FrameMapRelation{}, false
	}
	sourceTimestamp, err := strconv.ParseUint(sourceTSRaw, 10, 64)
	if err != nil {
		return FrameMapRelation{}, false
	}
	relation := FrameMapRelation{
		TimestampNS:       timestamp,
		RelationID:        uint32(relationID),
		SourceRow:         uint32(sourceRow),
		DestinationRow:    uint32(destinationRow),
		SourceTimestampNS: sourceTimestamp,
	}
	return relation, validFrameMapRelation(relation)
}

func validFrameMapRelation(relation FrameMapRelation) bool {
	return relation.SourceRow != relation.DestinationRow
}

func frameMapRelationEvent(lineNo int, relation FrameMapRelation, intern *stringInterner) Event {
	return Event{
		Line: lineNo,
		Ts:   float64(relation.TimestampNS) / 1e9,
		CPU:  -1,
		Type: EventFrameMap,
		Name: intern.intern("codrax_frame_map"),
		PluginFields: &PluginFields{
			FrameMap: &FrameMapFields{
				RelationID:             relation.RelationID,
				SourceRow:              relation.SourceRow,
				DestinationRow:         relation.DestinationRow,
				SourceTimestampNS:      relation.SourceTimestampNS,
				DestinationTimestampNS: relation.TimestampNS,
			},
		},
		FieldText: intern.intern(fmt.Sprintf(
			"relation_id=%d src_row=%d dst_row=%d src_ts_ns=%d dst_ts_ns=%d",
			relation.RelationID, relation.SourceRow, relation.DestinationRow,
			relation.SourceTimestampNS, relation.TimestampNS,
		)),
	}
}
