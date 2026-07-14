package hitraceconv

import (
	"errors"
	"fmt"
)

const traceStreamerTimestampCompanionName = sealedTraceDBVirtualName + ".ohos.ts"

var errTraceStreamerDBAuxiliaryState = errors.New("trace_streamer SQLite auxiliary state is not allowed")

var traceStreamerSQLiteAuxiliaryNames = [...]string{
	sealedTraceDBVirtualName + "-journal",
	sealedTraceDBVirtualName + "-wal",
	sealedTraceDBVirtualName + "-shm",
}

// sealedTraceStreamerDBOutputs freezes every trace_streamer output that can
// affect normalization. The SQLite main DB is consumed through its held
// handle. The optional timestamp companion is not parsed here, but its
// presence and generation must remain stable while the DB is queried.
type sealedTraceStreamerDBOutputs struct {
	dir              *privateConversionDir
	main             *sealedConversionFile
	companion        *sealedConversionFile
	companionPresent bool
}

func adoptTraceStreamerDBOutputs(dir *privateConversionDir) (*sealedTraceStreamerDBOutputs, error) {
	if dir == nil {
		return nil, fmt.Errorf("trace_streamer staging directory authority is missing")
	}
	if err := rejectTraceStreamerSQLiteAuxiliaryState(dir); err != nil {
		return nil, err
	}
	main, err := dir.AdoptRegularChild(sealedTraceDBVirtualName, true)
	if err != nil {
		return nil, fmt.Errorf("adopt trace_streamer SQLite main DB: %w", err)
	}
	outputs := &sealedTraceStreamerDBOutputs{dir: dir, main: main}
	companion, found, companionErr := dir.TryAdoptRegularChild(traceStreamerTimestampCompanionName, false)
	if companionErr == nil {
		outputs.companion = companion
		outputs.companionPresent = found
	}
	if companionErr != nil {
		return nil, traceDBJoinPreservingSingle(
			fmt.Errorf("adopt trace_streamer timestamp companion: %w", companionErr), outputs.close(),
		)
	}
	if err := rejectTraceStreamerSQLiteAuxiliaryState(dir); err != nil {
		return nil, traceDBJoinPreservingSingle(err, outputs.close())
	}
	return outputs, nil
}

func (outputs *sealedTraceStreamerDBOutputs) Size() int64 {
	if outputs == nil || outputs.main == nil {
		return 0
	}
	return outputs.main.Size()
}

func (outputs *sealedTraceStreamerDBOutputs) CompanionPresent() bool {
	return outputs != nil && outputs.companionPresent
}

func (outputs *sealedTraceStreamerDBOutputs) validate() error {
	if outputs == nil || outputs.dir == nil || outputs.main == nil {
		return fmt.Errorf("sealed trace_streamer DB output authority is incomplete")
	}
	if err := outputs.main.Validate(); err != nil {
		return fmt.Errorf("validate trace_streamer SQLite main DB: %w", err)
	}
	if outputs.companionPresent {
		if outputs.companion == nil {
			return fmt.Errorf("sealed trace_streamer timestamp companion authority is incomplete")
		}
		if err := outputs.companion.Validate(); err != nil {
			return fmt.Errorf("validate trace_streamer timestamp companion: %w", err)
		}
	} else {
		unexpected, found, err := outputs.dir.TryAdoptRegularChild(traceStreamerTimestampCompanionName, false)
		if unexpected != nil {
			err = traceDBJoinPreservingSingle(err, unexpected.Close())
		}
		if err != nil {
			return fmt.Errorf("confirm absent trace_streamer timestamp companion: %w", err)
		}
		if found {
			return fmt.Errorf("trace_streamer timestamp companion appeared after output adoption")
		}
	}
	return rejectTraceStreamerSQLiteAuxiliaryState(outputs.dir)
}

func (outputs *sealedTraceStreamerDBOutputs) finish(operationErr error) error {
	if outputs == nil {
		return traceDBJoinPreservingSingle(fmt.Errorf("sealed trace_streamer DB output authority is nil"), operationErr)
	}
	validationErr := outputs.validate()
	closeErr := outputs.close()
	return traceDBJoinPreservingSingle(validationErr, operationErr, closeErr)
}

func (outputs *sealedTraceStreamerDBOutputs) close() error {
	if outputs == nil {
		return nil
	}
	companionErr := outputs.companion.Close()
	mainErr := outputs.main.Close()
	outputs.companion = nil
	outputs.main = nil
	return traceDBJoinPreservingSingle(companionErr, mainErr)
}

func rejectTraceStreamerSQLiteAuxiliaryState(dir *privateConversionDir) error {
	if dir == nil {
		return fmt.Errorf("trace_streamer staging directory authority is missing")
	}
	for _, name := range traceStreamerSQLiteAuxiliaryNames {
		sealed, found, err := dir.TryAdoptRegularChild(name, false)
		if sealed != nil {
			err = traceDBJoinPreservingSingle(err, sealed.Close())
		}
		if err != nil {
			return fmt.Errorf("inspect trace_streamer SQLite auxiliary %q: %w", name, err)
		}
		if found {
			return fmt.Errorf("%w: %s", errTraceStreamerDBAuxiliaryState, name)
		}
	}
	return nil
}
