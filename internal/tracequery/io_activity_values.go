package tracequery

import "math"

var ioActivitySizeBounds = [...]uint64{0, 4096, 16384, 65536, 262144, 1048576}

type ioActivityValueAccumulator struct {
	events, known, unknown, invalid, overflow int
	sum                                       uint64
	sumOverflow                               bool
	sizes                                     [6]int
}

func (a *ioActivityValueAccumulator) add(item *ioActivityEndpoint) {
	a.events++
	switch item.byteStatus {
	case ioActivityBytesKnown:
		a.known++
		if math.MaxUint64-a.sum < item.bytes {
			a.sumOverflow = true
		} else if !a.sumOverflow {
			a.sum += item.bytes
		}
		bin := len(ioActivitySizeBounds) - 1
		for bin > 0 && item.bytes < ioActivitySizeBounds[bin] {
			bin--
		}
		a.sizes[bin]++
	case ioActivityBytesInvalid:
		a.invalid++
	case ioActivityBytesOverflow:
		a.overflow++
	default:
		a.unknown++
	}
}

func (a *ioActivityValueAccumulator) finish() IOActivityValues {
	out := IOActivityValues{
		EventCount: a.events, KnownByteEventCount: a.known, UnknownByteEventCount: a.unknown,
		InvalidByteEventCount: a.invalid, OverflowByteEventCount: a.overflow, BytesOverflow: a.sumOverflow,
	}
	// An empty bucket is a known empty event population. An unknown-size
	// nonempty population cannot borrow that zero-byte interpretation.
	if !a.sumOverflow && (a.known > 0 || a.events == 0) {
		total := a.sum
		out.KnownBytes = &total
		if a.known > 0 {
			mean := float64(total) / float64(a.known)
			out.KnownSizeMeanBytes = &mean
		}
	}
	for i, start := range ioActivitySizeBounds {
		bucket := IOActivitySizeBucket{MinBytes: start, Count: a.sizes[i]}
		if i+1 < len(ioActivitySizeBounds) {
			end := ioActivitySizeBounds[i+1]
			bucket.MaxBytes = &end
		}
		out.SizeBuckets = append(out.SizeBuckets, bucket)
	}
	return out
}

func ioActivityRates(values IOActivityValues, width float64) *IOActivityRates {
	if width <= 0 || !ioInFlightFinite(width) {
		return nil
	}
	events := float64(values.EventCount) / width
	if !ioInFlightFinite(events) {
		return nil
	}
	out := &IOActivityRates{EventsPerSecond: events}
	if values.KnownBytes != nil {
		bytes := float64(*values.KnownBytes) / width
		if ioInFlightFinite(bytes) {
			out.KnownBytesPerSecond = &bytes
		}
	}
	return out
}

type ioActivityPopulationAccumulator struct {
	total      ioActivityValueAccumulator
	directions [3]ioActivityValueAccumulator
}

var ioActivityDirections = [...]string{"read", "write", "other"}

func (a *ioActivityPopulationAccumulator) add(item *ioActivityEndpoint) {
	a.total.add(item)
	i := 2
	if item.direction == "read" {
		i = 0
	} else if item.direction == "write" {
		i = 1
	}
	a.directions[i].add(item)
}

func (a *ioActivityPopulationAccumulator) finishDirections(width float64) []IOActivityDirection {
	out := make([]IOActivityDirection, 0, len(ioActivityDirections))
	for i, direction := range ioActivityDirections {
		values := a.directions[i].finish()
		out = append(out, IOActivityDirection{Direction: direction, Values: values, Rates: ioActivityRates(values, width)})
	}
	return out
}

func (a *ioActivityPopulationAccumulator) readWriteRatio() *IOActivityReadWriteRatio {
	r, w := &a.directions[0], &a.directions[1]
	out := &IOActivityReadWriteRatio{EventDenominator: r.events + w.events}
	if out.EventDenominator > 0 {
		ratioR := float64(r.events) / float64(out.EventDenominator)
		ratioW := float64(w.events) / float64(out.EventDenominator)
		out.ReadEventShare, out.WriteEventShare = &ratioR, &ratioW
	}
	if r.known+w.known > 0 && !r.sumOverflow && !w.sumOverflow && math.MaxUint64-r.sum >= w.sum {
		denominator := r.sum + w.sum
		out.KnownByteDenominator = &denominator
		if denominator > 0 {
			ratioR, ratioW := float64(r.sum)/float64(denominator), float64(w.sum)/float64(denominator)
			out.ReadKnownByteShare, out.WriteKnownByteShare = &ratioR, &ratioW
		}
	}
	return out
}
