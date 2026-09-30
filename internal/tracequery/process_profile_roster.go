package tracequery

// Native header TGID or an already-validated scheduler head is identity
// evidence. A process name, marker payload PID or a majority vote is not.
func processProfileNativeRoster(idx *Index, q Query) (map[int]ThreadRef, map[int]bool) {
	roster, conflicts := map[int]ThreadRef{}, map[int]bool{}
	add := func(ref ThreadRef) {
		if ref.PID <= 0 {
			return
		}
		old := roster[ref.PID]
		if old.TGID > 0 && ref.TGID > 0 && old.TGID != ref.TGID {
			conflicts[ref.PID] = true
		}
		if ref.TGID == 0 {
			ref.TGID = old.TGID
		}
		if ref.Comm == "" {
			ref.Comm = old.Comm
		}
		roster[ref.PID] = ref
	}
	if head := schedulerHeadForQuery(idx, q); head != nil && head.Complete {
		for _, s := range head.Threads {
			add(s.Thread)
		}
	}
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			break
		}
		if !eventLineInWindow(ev, q) || ev.Ts < q.TimeStart || ev.Ts >= q.TimeEnd {
			continue
		}
		add(ThreadRef{PID: ev.PID, TGID: ev.TGID, Comm: ev.Comm})
		add(ThreadRef{PID: ev.PrevPID, Comm: ev.PrevComm})
		add(ThreadRef{PID: ev.NextPID, Comm: ev.NextComm})
		add(ThreadRef{PID: ev.WakeePID, Comm: ev.WakeeComm})
	}
	return roster, conflicts
}
