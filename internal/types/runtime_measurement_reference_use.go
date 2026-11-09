package types

// An interpretation boundary is producer teaching, not new evidence or a
// hard check on model prose. The current source/row binder owns when it can
// accompany a reference. Unknown producer semantics keep the legacy path.
type runtimeMeasurementReferenceUse uint8

const (
	runtimeMeasurementReferenceUnspecified runtimeMeasurementReferenceUse = iota
	runtimeMeasurementReferenceSchedulerPopulation
	runtimeMeasurementReferenceThreadStates
	runtimeMeasurementReferenceSamples
)

func (c runtimeMeasurementProducerContract) referenceUse() runtimeMeasurementReferenceUse {
	switch c.predicate {
	case "scheduler_concurrency":
		return runtimeMeasurementReferenceSchedulerPopulation
	case "thread_timeline_observation":
		return runtimeMeasurementReferenceThreadStates
	case RuntimeSamplingMeasurementPredicate:
		return runtimeMeasurementReferenceSamples
	default:
		return runtimeMeasurementReferenceUnspecified
	}
}

func (u runtimeMeasurementReferenceUse) limit() string {
	switch u {
	case runtimeMeasurementReferenceSchedulerPopulation:
		return "Closed system-wide intervals; runnable is not running. Zero contribution is not absence. Peak or busy time does not prove sustained saturation or a thread cause."
	case runtimeMeasurementReferenceThreadStates:
		return "Only this thread's observed states. Running, runnable (waiting for CPU), sleep, D and I/O wait are distinct; zero in one cannot rule out another. Wakeup is not execution."
	case runtimeMeasurementReferenceSamples:
		return "Only recorded sample points and weights. Unknown sample type does not establish running; no sample is not inactivity. Samples do not prove duration, a call, or a cause."
	default:
		return ""
	}
}
