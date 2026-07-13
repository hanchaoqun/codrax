package hitraceconv

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectBlockElapsedRegistryAndFormatProvenanceAreExact(t *testing.T) {
	elapsed := []string{
		"block_bio_queue",
		"block_bio_complete",
		"block_rq_issue",
		"block_rq_complete",
	}
	for _, name := range elapsed {
		if !directBlockNameGoverned(name) || !directBlockPairEndpointName(name) ||
			pairCriticalFormatFamilyForName(name) != pairCriticalFormatFamilyBlock {
			t.Fatalf("exact Block endpoint lost direct/barrier authority: name=%q governed=%t endpoint=%t family=%d",
				name, directBlockNameGoverned(name), directBlockPairEndpointName(name), pairCriticalFormatFamilyForName(name))
		}
	}

	inventory := []string{"block_bio_remap", "block_rq_insert", "block_rq_remap"}
	for _, name := range inventory {
		if !directBlockNameGoverned(name) || directBlockPairEndpointName(name) || pairCriticalFormatFamilyForName(name) != 0 {
			t.Fatalf("Block inventory crossed the elapsed barrier: name=%q governed=%t endpoint=%t family=%d",
				name, directBlockNameGoverned(name), directBlockPairEndpointName(name), pairCriticalFormatFamilyForName(name))
		}
	}

	for _, exact := range append(append([]string(nil), elapsed...), inventory...) {
		for _, near := range []string{
			strings.ToUpper(exact),
			" " + exact,
			exact + " ",
			exact + "_vendor",
		} {
			if directBlockNameGoverned(near) || directBlockPairEndpointName(near) || pairCriticalFormatFamilyForName(near) != 0 {
				t.Fatalf("near Block name gained direct/barrier authority: exact=%q near=%q", exact, near)
			}
		}
	}

	issue := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
		dev: 8 << 20, sector: 1, nrSector: 8, bytes: 4096, rwbs: "R",
	})
	for index, test := range []struct {
		rawName string
		want    string
	}{
		{rawName: "name:  block_rq_issue", want: " block_rq_issue"},
		{rawName: "name: block_rq_issue ", want: "block_rq_issue "},
		{rawName: "name: BLOCK_RQ_ISSUE", want: "BLOCK_RQ_ISSUE"},
		{rawName: "name: block_rq_issue_vendor", want: "block_rq_issue_vendor"},
	} {
		id := 690 + index
		body := directPairFormatBlock(id, issue.format)
		body = strings.Replace(body, "name: block_rq_issue", test.rawName, 1)
		catalog, err := parseEventFormats([]byte(body))
		candidate, present := catalog.Formats[id]
		if err != nil || !present || catalog.Poisoned[id] || candidate.Name != test.want ||
			directBlockNameGoverned(candidate.Name) || pairCriticalFormatFamilyForName(candidate.Name) != 0 {
			t.Fatalf("parsed near-name gained exact Block authority: raw=%q want=%q candidate=%+v present=%t poisoned=%t families=%v err=%v",
				test.rawName, test.want, candidate, present, catalog.Poisoned[id], catalog.PoisonedFamilies, err)
		}
	}
	crlfCatalog, err := parseEventFormats([]byte(strings.ReplaceAll(directPairFormatBlock(699, issue.format), "\n", "\r\n")))
	if candidate, present := crlfCatalog.Formats[699]; err != nil || !present || candidate.Name != "block_rq_issue" {
		t.Fatalf("CRLF syntax changed an exact Block name: candidate=%+v present=%t err=%v", candidate, present, err)
	}
	complete := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: 8 << 20, sector: 1, nrSector: 8, rwbs: "R",
	})
	exactCatalog, err := parseEventFormats([]byte(strings.Join([]string{
		directPairFormatBlock(701, issue.format),
		directPairFormatBlock(701, complete.format),
	}, "\n")))
	if err != nil || !exactCatalog.Poisoned[701] ||
		exactCatalog.PoisonedFamilies[701]&pairCriticalFormatFamilyBlock == 0 {
		t.Fatalf("conflicting exact Block descriptor lost family provenance: poisoned=%v families=%v err=%v",
			exactCatalog.Poisoned, exactCatalog.PoisonedFamilies, err)
	}

	insert := directBlockPairFixture("block_rq_insert", 100, directBlockFixtureValues{
		dev: 8 << 20, sector: 1, nrSector: 8, bytes: 4096, rwbs: "R",
	})
	remap := directBlockPairFixture("block_rq_remap", 100, directBlockFixtureValues{
		dev: 8 << 20, sector: 1, nrSector: 8, oldDev: 1, oldSector: 2, nrBios: 1, rwbs: "R",
	})
	inventoryCatalog, err := parseEventFormats([]byte(strings.Join([]string{
		directPairFormatBlock(702, insert.format),
		directPairFormatBlock(702, remap.format),
	}, "\n")))
	if err != nil || !inventoryCatalog.Poisoned[702] || inventoryCatalog.PoisonedFamilies[702] != 0 {
		t.Fatalf("inventory descriptor conflict gained Block elapsed provenance: poisoned=%v families=%v err=%v",
			inventoryCatalog.Poisoned, inventoryCatalog.PoisonedFamilies, err)
	}
}

func TestDirectBlockOwnerUnknownIsFamilyScopedButKnownIdleAndCrossEmitterShareLane(t *testing.T) {
	values := directBlockFixtureValues{
		dev: 8 << 20, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	}

	t.Run("unknown owner is not idle", func(t *testing.T) {
		fixture := directBlockPairFixture("block_rq_issue", -1, values)
		_, _, _, envelopeOK, audit := renderEventLineDecisionWithPairAudit(
			renderContext{}, 1_000, 0, fixture.format, fixture.content,
		)
		if envelopeOK || !audit.Governed || audit.HeaderOwnerKnown || !audit.Verdict.KeyKnown ||
			audit.Verdict.EmitterKnown || audit.EndpointAdmitted {
			t.Fatalf("unknown Block owner was erased or fabricated as idle: envelope=%t audit=%+v", envelopeOK, audit)
		}
		barrier := newDirectBlockTestBarrier(t)
		barrier.observe(audit)
		if !barrier.poisonedKinds[pairRenderBlock] || barrier.poisonedLaneCount() != 0 {
			t.Fatalf("unknown owner did not close the source-local Block family: kinds=%v lanes=%v",
				barrier.poisonedKinds, barrier.poisonedLanes)
		}
	})

	t.Run("known emitters do not enter request identity", func(t *testing.T) {
		barrier := newDirectBlockTestBarrier(t)
		for _, owners := range [][2]int32{{0, 2}, {100, 2}} {
			issue := directBlockAdmittedAudit(t, "block_rq_issue", owners[0], values)
			doneValues := values
			doneValues.bytes = 0
			done := directBlockAdmittedAudit(t, "block_rq_complete", owners[1], doneValues)
			issueLane, issueOK := pairingEndpointLaneKey(issue.Verdict, barrier.source)
			doneLane, doneOK := pairingEndpointLaneKey(done.Verdict, barrier.source)
			if !issueOK || !doneOK || issueLane != doneLane {
				t.Fatalf("known emitter entered Block request identity: owners=%v issue=%q/%t done=%q/%t",
					owners, issueLane, issueOK, doneLane, doneOK)
			}
		}
	})

	t.Run("known idle to positive pairs end to end", func(t *testing.T) {
		issue := directBlockPairFixture("block_rq_issue", 0, values)
		done := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
			dev: values.dev, sector: values.sector, nrSector: values.nrSector, rwbs: values.rwbs,
		})
		result, _, output := convertDirectPairCapture(t,
			[]directPairFormatSpec{{id: 711, format: issue.format}, {id: 712, format: done.format}},
			[]syntheticRawEvent{
				{EventID: 711, OffsetNS: 1_000, Content: issue.content},
				{EventID: 712, OffsetNS: 2_000, Content: done.content},
			},
		)
		assertDirectBlockBarrierCoverage(t, result, 2, 2, 0, 2)
		stats := directPairWindowStats(t, output)
		if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].IssueThread.PID != 0 ||
			stats.IOLatencies[0].CompleteThread.PID != 2 {
			t.Fatalf("known idle Block endpoint failed to pair across context: %+v caveats=%v",
				stats.IOLatencies, stats.Caveats)
		}
	})
}

func TestDirectBlockHardKeyFailuresAreFamilyScopedAndNonKeyFailuresStayExact(t *testing.T) {
	baseValues := directBlockFixtureValues{
		dev: 8 << 20, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	}
	hardKeyCases := []struct {
		name   string
		mutate func(*directPairTestFixture)
	}{
		{name: "dev", mutate: func(fixture *directPairTestFixture) {
			fixture.format.Fields = directFieldsWithoutCleanName(fixture.format.Fields, "dev")
		}},
		{name: "sector", mutate: func(fixture *directPairTestFixture) {
			fixture.format.Fields = directFieldsWithoutCleanName(fixture.format.Fields, "sector")
		}},
		{name: "nr_sector", mutate: func(fixture *directPairTestFixture) {
			fixture.format.Fields = directFieldsWithoutCleanName(fixture.format.Fields, "nr_sector")
		}},
		{name: "op", mutate: func(fixture *directPairTestFixture) {
			rwbs := directPairField(t, fixture, "rwbs")
			copy(fixture.content[rwbs.Offset:rwbs.Offset+rwbs.Size], make([]byte, rwbs.Size))
			copy(fixture.content[rwbs.Offset:rwbs.Offset+rwbs.Size], []byte("R|W"))
		}},
	}
	for _, test := range hardKeyCases {
		t.Run("hard key "+test.name, func(t *testing.T) {
			fixture := directBlockPairFixture("block_rq_issue", 100, baseValues)
			test.mutate(&fixture)
			decision := decodeDirectBlockPayloadDecision(decodeEvent(fixture.format, fixture.content), fixture.content)
			if decision.Admission != bodyRejected || decision.IdentityKnown {
				t.Fatalf("bad Block hard key retained identity: field=%s decision=%+v", test.name, decision)
			}
			audit := newDirectBlockLineAudit(decodeEvent(fixture.format, fixture.content), decision)
			barrier := newDirectBlockTestBarrier(t)
			barrier.observe(audit)
			if !barrier.poisonedKinds[pairRenderBlock] || barrier.poisonedLaneCount() != 0 {
				t.Fatalf("unknown Block hard key did not close its family: field=%s kinds=%v lanes=%v",
					test.name, barrier.poisonedKinds, barrier.poisonedLanes)
			}
		})
	}

	nonKeyCases := []struct {
		name    string
		event   string
		values  directBlockFixtureValues
		field   string
		sibling directBlockFixtureValues
	}{
		{
			name: "bytes", event: "block_rq_issue", values: baseValues, field: "bytes",
			sibling: directBlockFixtureValues{dev: baseValues.dev, sector: 456, nrSector: 8, bytes: 4096, rwbs: "R"},
		},
		{
			name: "error", event: "block_bio_complete", field: "error",
			values:  directBlockFixtureValues{dev: baseValues.dev, sector: baseValues.sector, nrSector: 8, rwbs: "R"},
			sibling: directBlockFixtureValues{dev: baseValues.dev, sector: 456, nrSector: 8, rwbs: "R"},
		},
	}
	for _, test := range nonKeyCases {
		t.Run("non-key "+test.name, func(t *testing.T) {
			bad := directBlockPairFixture(test.event, 100, test.values)
			directSetField(&bad.format, test.field, func(field *eventField) { field.Signed = !field.Signed })
			ev := decodeEvent(bad.format, bad.content)
			decision := decodeDirectBlockPayloadDecision(ev, bad.content)
			if decision.Admission != bodyRejected || !decision.IdentityKnown {
				t.Fatalf("bad Block non-key lost exact identity: field=%s decision=%+v", test.name, decision)
			}
			badAudit := newDirectBlockLineAudit(ev, decision)
			barrier := newDirectBlockTestBarrier(t)
			barrier.observe(badAudit)
			if barrier.poisonedKinds[pairRenderBlock] || barrier.poisonedLaneCount() != 1 {
				t.Fatalf("bad Block non-key widened beyond its exact lane: field=%s kinds=%v lanes=%v",
					test.name, barrier.poisonedKinds, barrier.poisonedLanes)
			}

			sibling := directBlockAdmittedAudit(t, test.event, 100, test.sibling)
			barrier.observe(sibling)
			barrier.addPublishedRow(1, sibling)
			filtered := barrier.filter([]renderedRow{{seq: 1, pairKind: pairRenderBlock, line: "sibling"}})
			if len(filtered) != 1 || filtered[0].line != "sibling" {
				t.Fatalf("exact-lane poison damaged a different Block key: field=%s filtered=%+v", test.name, filtered)
			}
		})
	}

	t.Run("comm and cmd are display-only", func(t *testing.T) {
		issue := directBlockPairFixture("block_rq_issue", 100, baseValues)
		for _, fieldName := range []string{"comm", "cmd"} {
			directSetField(&issue.format, fieldName, func(field *eventField) { field.Type = "unsigned int" })
		}
		decision := decodeDirectBlockPayloadDecision(decodeEvent(issue.format, issue.content), issue.content)
		if decision.Admission != bodyAdmitted || !decision.IdentityKnown || decision.Payload.comm != "" || decision.Payload.cmd != "" ||
			!stringSliceContains(decision.Degradations, "direct_comm_wrong_type") ||
			!stringSliceContains(decision.Degradations, "direct_cmd_wrong_type") {
			t.Fatalf("display-only Block fields changed endpoint admission: %+v", decision)
		}
		done := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
			dev: baseValues.dev, sector: baseValues.sector, nrSector: baseValues.nrSector, rwbs: baseValues.rwbs,
		})
		result, body, output := convertDirectPairCapture(t,
			[]directPairFormatSpec{{id: 721, format: issue.format}, {id: 722, format: done.format}},
			[]syntheticRawEvent{
				{EventID: 721, OffsetNS: 1_000, Content: issue.content},
				{EventID: 722, OffsetNS: 2_000, Content: done.content},
			},
		)
		if !strings.Contains(body, "block_rq_issue: 8,0 R 4096 () 123 + 8 []") {
			t.Fatalf("display omission changed canonical Block grammar:\n%s", body)
		}
		assertDirectBlockBarrierCoverage(t, result, 2, 2, 0, 2)
		if stats := directPairWindowStats(t, output); len(stats.IOLatencies) != 1 {
			t.Fatalf("display-only omission poisoned Block pairing: latencies=%+v caveats=%v", stats.IOLatencies, stats.Caveats)
		}
	})
}

func TestDirectBlockDescriptorOverlapCannotMintRequestIdentity(t *testing.T) {
	values := directBlockFixtureValues{
		dev: 123, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	}
	for _, test := range []struct {
		name   string
		mutate func(*directPairTestFixture)
	}{
		{
			name: "dev and sector",
			mutate: func(fixture *directPairTestFixture) {
				devOffset := directPairField(t, fixture, "dev").Offset
				directPairField(t, fixture, "sector").Offset = devOffset
			},
		},
		{
			name: "sector and nr_sector",
			mutate: func(fixture *directPairTestFixture) {
				sectorOffset := directPairField(t, fixture, "sector").Offset
				directPairField(t, fixture, "nr_sector").Offset = sectorOffset
			},
		},
		{
			name: "rwbs locator and nr_sector",
			mutate: func(fixture *directPairTestFixture) {
				nrSectorOffset := directPairField(t, fixture, "nr_sector").Offset
				rwbs := directPairField(t, fixture, "rwbs")
				rwbs.Type, rwbs.Name, rwbs.Offset, rwbs.Size = "__data_loc char[]", "rwbs", nrSectorOffset, 4
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := directBlockPairFixture("block_rq_issue", 100, values)
			test.mutate(&fixture)
			ev := decodeEvent(fixture.format, fixture.content)
			decision := decodeDirectBlockPayloadDecision(ev, fixture.content)
			if decision.Admission != bodyRejected || decision.IdentityKnown ||
				!strings.Contains(decision.Reason, "overlapping_field") {
				t.Fatalf("overlapping Block descriptor minted a request identity: decision=%+v format=%+v", decision, fixture.format)
			}
			barrier := newDirectBlockTestBarrier(t)
			barrier.observe(newDirectBlockLineAudit(ev, decision))
			if !barrier.poisonedKinds[pairRenderBlock] || barrier.poisonedLaneCount() != 0 {
				t.Fatalf("unknown overlap did not fail-close the Block family: kinds=%v lanes=%v",
					barrier.poisonedKinds, barrier.poisonedLanes)
			}
			work := directPairAdmittedAudit(t,
				directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xaaa, 0x111))
			barrier.observe(work)
			barrier.addPublishedRow(901, work)
			rows := barrier.filter([]renderedRow{{seq: 901, pairKind: pairRenderWorkqueue, line: "work"}})
			if len(rows) != 1 || rows[0].line != "work" || barrier.poisonedKinds[pairRenderWorkqueue] {
				t.Fatalf("Block descriptor overlap crossed into the legacy proof domain: rows=%+v kinds=%v", rows, barrier.poisonedKinds)
			}
			if err := barrier.validateAccounting(rows); err != nil {
				t.Fatalf("Block overlap/legacy conservation failed: %v", err)
			}
		})
	}

	t.Run("converter to tracequery cannot self-consistently wash overlap", func(t *testing.T) {
		issue := directBlockPairFixture("block_rq_issue", 100, values)
		badDone := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
			dev: values.dev, sector: values.sector, nrSector: values.nrSector, rwbs: values.rwbs,
		})
		devOffset := directPairField(t, &badDone, "dev").Offset
		directPairField(t, &badDone, "sector").Offset = devOffset
		goodDone := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
			dev: values.dev, sector: values.sector, nrSector: values.nrSector, rwbs: values.rwbs,
		})
		result, body, output := convertDirectPairCapture(t,
			[]directPairFormatSpec{
				{id: 751, format: issue.format}, {id: 752, format: badDone.format}, {id: 753, format: goodDone.format},
			},
			[]syntheticRawEvent{
				{EventID: 751, OffsetNS: 1_000, Content: issue.content},
				{EventID: 752, OffsetNS: 2_000, Content: badDone.content},
				{EventID: 753, OffsetNS: 3_000, Content: goodDone.content},
			},
		)
		if strings.Contains(body, "block_rq_issue:") || strings.Contains(body, "block_rq_complete:") {
			t.Fatalf("overlap was washed through canonical typed/wire parity:\n%s", body)
		}
		assertDirectPairBarrierCaveat(t, result, "poisoned_families=1", "withheld_rows=2")
		assertDirectBlockBarrierCoverage(t, result, 3, 2, 2, 0)
		if stats := directPairWindowStats(t, output); len(stats.IOLatencies) != 0 {
			t.Fatalf("overlapping descriptor minted a false I/O duration: %+v", stats.IOLatencies)
		}
	})
}

func TestDirectBlockDataLocMustFollowFixedTailAndRemainPhysicallyDisjoint(t *testing.T) {
	values := directBlockFixtureValues{
		dev: 8 << 20, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	}

	t.Run("valid target after fixed tail", func(t *testing.T) {
		fixture := directBlockPairFixture("block_rq_issue", 100, values)
		offset := len(fixture.content)
		fixture.content = append(fixture.content, 'R', 0)
		directBlockSetDataLoc(t, &fixture, "rwbs", offset, 2)
		decision := decodeDirectBlockPayloadDecision(decodeEvent(fixture.format, fixture.content), fixture.content)
		if decision.Admission != bodyAdmitted || !decision.IdentityKnown || decision.Payload.rwbs != "R" {
			t.Fatalf("valid post-tail Block data_loc was rejected: %+v", decision)
		}
	})

	t.Run("target back into fixed bytes", func(t *testing.T) {
		fixture := directBlockPairFixture("block_rq_issue", 100, values)
		commOffset := directPairField(t, &fixture, "comm").Offset
		copy(fixture.content[commOffset:], []byte{'R', 0})
		directBlockSetDataLoc(t, &fixture, "rwbs", commOffset, 2)
		decision := decodeDirectBlockPayloadDecision(decodeEvent(fixture.format, fixture.content), fixture.content)
		if decision.Admission != bodyRejected || decision.IdentityKnown {
			t.Fatalf("Block data_loc back-reference minted hard-key provenance: %+v", decision)
		}
	})

	t.Run("optional range cannot alias hard rwbs", func(t *testing.T) {
		fixture := directBlockPairFixture("block_rq_issue", 100, values)
		offset := len(fixture.content)
		fixture.content = append(fixture.content, 'R', 0)
		directBlockSetDataLoc(t, &fixture, "rwbs", offset, 2)
		directBlockSetDataLoc(t, &fixture, "comm", offset, 2)
		decision := decodeDirectBlockPayloadDecision(decodeEvent(fixture.format, fixture.content), fixture.content)
		if decision.Admission != bodyRejected || decision.IdentityKnown || decision.Reason != "direct_rwbs_dynamic_overlap" {
			t.Fatalf("aliased dynamic display field laundered hard rwbs provenance: %+v", decision)
		}
	})

	t.Run("display-only overlap degrades without widening", func(t *testing.T) {
		fixture := directBlockPairFixture("block_rq_issue", 100, values)
		offset := len(fixture.content)
		fixture.content = append(fixture.content, 'i', 'o', 0)
		directBlockSetDataLoc(t, &fixture, "cmd", offset, 3)
		directBlockSetDataLoc(t, &fixture, "comm", offset, 3)
		decision := decodeDirectBlockPayloadDecision(decodeEvent(fixture.format, fixture.content), fixture.content)
		if decision.Admission != bodyAdmitted || !decision.IdentityKnown || decision.Payload.cmd != "" ||
			decision.Payload.comm != "" || !stringSliceContains(decision.Degradations, "direct_cmd_dynamic_overlap") ||
			!stringSliceContains(decision.Degradations, "direct_comm_dynamic_overlap") {
			t.Fatalf("display-only alias changed Block request admission: %+v", decision)
		}
	})
}

func TestDirectBlockNumericArrayDeclaratorsCannotBecomeScalars(t *testing.T) {
	values := directBlockFixtureValues{
		dev: 8 << 20, sector: 123, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	}
	for _, fieldName := range []string{"dev", "sector", "nr_sector"} {
		t.Run("hard key "+fieldName, func(t *testing.T) {
			fixture := directBlockPairFixture("block_rq_issue", 100, values)
			field := directPairField(t, &fixture, fieldName)
			field.Name = fieldName + "[1]"
			ev := decodeEvent(fixture.format, fixture.content)
			decision := decodeDirectBlockPayloadDecision(ev, fixture.content)
			if decision.Admission != bodyRejected || decision.IdentityKnown || decision.Reason != "direct_"+fieldName+"_wrong_type" {
				t.Fatalf("hard-key array declarator became a scalar: field=%s decision=%+v", fieldName, decision)
			}
			barrier := newDirectBlockTestBarrier(t)
			barrier.observe(newDirectBlockLineAudit(ev, decision))
			if !barrier.poisonedKinds[pairRenderBlock] || barrier.poisonedLaneCount() != 0 {
				t.Fatalf("unknown hard-key array did not close Block family: field=%s kinds=%v lanes=%v",
					fieldName, barrier.poisonedKinds, barrier.poisonedLanes)
			}
		})
	}
	t.Run("hard-key array closes converter family", func(t *testing.T) {
		issue := directBlockPairFixture("block_rq_issue", 100, values)
		badDone := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
			dev: values.dev, sector: values.sector, nrSector: values.nrSector, rwbs: values.rwbs,
		})
		directPairField(t, &badDone, "sector").Name = "sector[1]"
		goodDone := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
			dev: values.dev, sector: values.sector, nrSector: values.nrSector, rwbs: values.rwbs,
		})
		result, body, output := convertDirectPairCapture(t,
			[]directPairFormatSpec{
				{id: 771, format: issue.format}, {id: 772, format: badDone.format}, {id: 773, format: goodDone.format},
			},
			[]syntheticRawEvent{
				{EventID: 771, OffsetNS: 1_000, Content: issue.content},
				{EventID: 772, OffsetNS: 2_000, Content: badDone.content},
				{EventID: 773, OffsetNS: 3_000, Content: goodDone.content},
			},
		)
		if strings.Contains(body, "block_rq_issue:") || strings.Contains(body, "block_rq_complete:") {
			t.Fatalf("hard-key array endpoint was rescued by canonical text:\n%s", body)
		}
		assertDirectPairBarrierCaveat(t, result, "poisoned_families=1", "withheld_rows=2")
		assertDirectBlockBarrierCoverage(t, result, 3, 2, 2, 0)
		if stats := directPairWindowStats(t, output); len(stats.IOLatencies) != 0 {
			t.Fatalf("hard-key array endpoint minted a false I/O pair: %+v", stats.IOLatencies)
		}
	})

	for index, test := range []struct {
		name      string
		eventName string
		fieldName string
	}{
		{name: "bytes", eventName: "block_rq_issue", fieldName: "bytes"},
		{name: "error", eventName: "block_rq_complete", fieldName: "error"},
	} {
		t.Run("non-key "+test.name, func(t *testing.T) {
			badValues := values
			if test.eventName == "block_rq_complete" {
				badValues.bytes, badValues.comm, badValues.cmd = 0, "", ""
			}
			bad := directBlockPairFixture(test.eventName, 2, badValues)
			field := directPairField(t, &bad, test.fieldName)
			field.Name = test.fieldName + "[1]"
			ev := decodeEvent(bad.format, bad.content)
			decision := decodeDirectBlockPayloadDecision(ev, bad.content)
			if decision.Admission != bodyRejected || !decision.IdentityKnown ||
				decision.Reason != "direct_"+test.fieldName+"_wrong_type" {
				t.Fatalf("non-key array declarator changed identity scope: field=%s decision=%+v", test.fieldName, decision)
			}
			barrier := newDirectBlockTestBarrier(t)
			barrier.observe(newDirectBlockLineAudit(ev, decision))
			if barrier.poisonedKinds[pairRenderBlock] || barrier.poisonedLaneCount() != 1 {
				t.Fatalf("non-key array widened beyond exact Block lane: field=%s kinds=%v lanes=%v",
					test.fieldName, barrier.poisonedKinds, barrier.poisonedLanes)
			}

			issue := directBlockPairFixture("block_rq_issue", 100, values)
			done := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
				dev: values.dev, sector: values.sector, nrSector: values.nrSector, rwbs: values.rwbs,
			})
			baseID := 761 + index*3
			result, body, output := convertDirectPairCapture(t,
				[]directPairFormatSpec{
					{id: baseID, format: issue.format}, {id: baseID + 1, format: bad.format}, {id: baseID + 2, format: done.format},
				},
				[]syntheticRawEvent{
					{EventID: uint16(baseID), OffsetNS: 1_000, Content: issue.content},
					{EventID: uint16(baseID + 1), OffsetNS: 2_000, Content: bad.content},
					{EventID: uint16(baseID + 2), OffsetNS: 3_000, Content: done.content},
				},
			)
			if strings.Contains(body, "block_rq_issue:") || strings.Contains(body, "block_rq_complete:") {
				t.Fatalf("array non-key endpoint was rescued by neighboring valid rows:\n%s", body)
			}
			assertDirectPairBarrierCaveat(t, result, "poisoned_families=0", "poisoned_lanes=1", "withheld_rows=2")
			assertDirectBlockBarrierCoverage(t, result, 3, 2, 2, 0)
			if stats := directPairWindowStats(t, output); len(stats.IOLatencies) != 0 {
				t.Fatalf("array non-key endpoint minted a false I/O pair: %+v", stats.IOLatencies)
			}
		})
	}
}

func directBlockSetDataLoc(t *testing.T, fixture *directPairTestFixture, fieldName string, offset, length int) {
	t.Helper()
	if offset < 0 || offset > 0xffff || length <= 0 || length > 0xffff {
		t.Fatalf("invalid synthetic data_loc range: offset=%d length=%d", offset, length)
	}
	field := directPairField(t, fixture, fieldName)
	field.Type = "__data_loc char[]"
	field.Name = fieldName
	field.Size = 4
	field.Signed = false
	if field.Offset < 0 || field.Offset > len(fixture.content)-4 {
		t.Fatalf("synthetic locator field is out of bounds: field=%+v len=%d", field, len(fixture.content))
	}
	binary.LittleEndian.PutUint32(fixture.content[field.Offset:field.Offset+4], uint32(offset)|uint32(length)<<16)
}

func TestDirectBlockZeroLengthFlushCaseIsAdmittedButLaneCaseIsPreserved(t *testing.T) {
	const dev = uint64(12<<20 | 80)
	barrier := newDirectBlockTestBarrier(t)
	lanes := make(map[string]string)
	for _, op := range []string{"F", "FS", "f", "fS"} {
		values := directBlockFixtureValues{dev: dev, sector: 0, nrSector: 0, bytes: 0, rwbs: op}
		audit := directBlockAdmittedAudit(t, "block_rq_issue", 100, values)
		lane, ok := pairingEndpointLaneKey(audit.Verdict, barrier.source)
		if !ok || lane == "" {
			t.Fatalf("zero-length flush lost exact lane: op=%q audit=%+v", op, audit)
		}
		if prior, exists := lanes[lane]; exists {
			t.Fatalf("case-preserved Block operations collided: prior=%q current=%q lane=%q", prior, op, lane)
		}
		lanes[lane] = op
	}
	if len(lanes) != 4 {
		t.Fatalf("zero-length flush lane cardinality drifted: %+v", lanes)
	}

	issueFormat := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
		dev: dev, sector: 0, nrSector: 0, bytes: 0, rwbs: "F",
	})
	doneFormat := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: dev, sector: 0, nrSector: 0, rwbs: "F",
	})
	var events []syntheticRawEvent
	for index, op := range []string{"F", "FS", "f", "fS"} {
		issue := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
			dev: dev, sector: 0, nrSector: 0, bytes: 0, rwbs: op,
		})
		done := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
			dev: dev, sector: 0, nrSector: 0, rwbs: op,
		})
		events = append(events,
			syntheticRawEvent{EventID: 731, OffsetNS: uint32(index*2+1) * 1_000, Content: issue.content},
			syntheticRawEvent{EventID: 732, OffsetNS: uint32(index*2+2) * 1_000, Content: done.content},
		)
	}
	result, _, output := convertDirectPairCapture(t,
		[]directPairFormatSpec{{id: 731, format: issueFormat.format}, {id: 732, format: doneFormat.format}},
		events,
	)
	assertDirectBlockBarrierCoverage(t, result, 8, 8, 0, 8)
	stats := directPairWindowStats(t, output)
	if len(stats.IOLatencies) != 4 {
		t.Fatalf("legal zero-length flush operations failed to pair independently: %+v caveats=%v", stats.IOLatencies, stats.Caveats)
	}
	seenOps := make(map[string]bool)
	for _, latency := range stats.IOLatencies {
		seenOps[latency.Op] = true
	}
	for _, op := range []string{"F", "FS", "f", "fS"} {
		if !seenOps[op] {
			t.Fatalf("zero-length flush operation disappeared or changed case: op=%q latencies=%+v", op, stats.IOLatencies)
		}
	}

	upperIssue := directBlockPairFixture("block_rq_issue", 100, directBlockFixtureValues{
		dev: dev, sector: 0, nrSector: 0, bytes: 0, rwbs: "F",
	})
	lowerDone := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: dev, sector: 0, nrSector: 0, rwbs: "f",
	})
	_, _, crossCaseOutput := convertDirectPairCapture(t,
		[]directPairFormatSpec{{id: 733, format: upperIssue.format}, {id: 734, format: lowerDone.format}},
		[]syntheticRawEvent{
			{EventID: 733, OffsetNS: 1_000, Content: upperIssue.content},
			{EventID: 734, OffsetNS: 2_000, Content: lowerDone.content},
		},
	)
	if crossCase := directPairWindowStats(t, crossCaseOutput); len(crossCase.IOLatencies) != 0 {
		t.Fatalf("cross-case zero-length endpoints were incorrectly paired: %+v", crossCase.IOLatencies)
	}
}

func TestDirectBlockInventoryOnlyDoesNotMintBarrierCoverage(t *testing.T) {
	const dev = uint64(8 << 20)
	insert := directBlockPairFixture("block_rq_insert", 100, directBlockFixtureValues{
		dev: dev, sector: 1, nrSector: 8, bytes: 4096, rwbs: "R", comm: "io", cmd: "READ",
	})
	bioRemap := directBlockPairFixture("block_bio_remap", 100, directBlockFixtureValues{
		dev: dev, sector: 2, nrSector: 8, oldDev: 1, oldSector: 20, rwbs: "R",
	})
	rqRemap := directBlockPairFixture("block_rq_remap", 100, directBlockFixtureValues{
		dev: dev, sector: 3, nrSector: 8, oldDev: 1, oldSector: 30, nrBios: 2, rwbs: "R",
	})
	result, body, output := convertDirectPairCapture(t,
		[]directPairFormatSpec{
			{id: 741, format: insert.format}, {id: 742, format: bioRemap.format}, {id: 743, format: rqRemap.format},
		},
		[]syntheticRawEvent{
			{EventID: 741, OffsetNS: 1_000, Content: insert.content},
			{EventID: 742, OffsetNS: 2_000, Content: bioRemap.content},
			{EventID: 743, OffsetNS: 3_000, Content: rqRemap.content},
		},
	)
	if result.EventsWritten != 3 || !strings.Contains(body, "block_rq_insert:") ||
		!strings.Contains(body, "block_bio_remap:") || !strings.Contains(body, "block_rq_remap:") {
		t.Fatalf("Block inventory was not preserved independently: result=%+v\n%s", result, body)
	}
	for _, coverage := range result.TraceCoverage {
		if coverage.Family == "builtin_raw_ftrace:block_capture" {
			t.Fatalf("inventory-only Block rows minted elapsed barrier coverage: %+v", coverage)
		}
	}
	if strings.Contains(strings.Join(result.Caveats, "\n"), "direct pair-critical publication") {
		t.Fatalf("inventory-only Block rows activated elapsed barrier caveat: %+v", result.Caveats)
	}
	if stats := directPairWindowStats(t, output); len(stats.IOLatencies) != 0 {
		t.Fatalf("Block inventory minted elapsed latency: %+v", stats.IOLatencies)
	}
}

func newDirectBlockTestBarrier(t *testing.T) *directPairCaptureBarrier {
	t.Helper()
	barrier, err := newDirectPairCaptureBarrier(filepath.Join(t.TempDir(), "direct-block.ftrace"))
	if err != nil {
		t.Fatal(err)
	}
	return barrier
}
