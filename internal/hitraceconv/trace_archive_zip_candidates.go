package hitraceconv

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
)

func selectTraceArchiveZIPMember(ctx context.Context, reader *zip.Reader, directory traceArchiveZIPDirectory, explicit string) (traceArchiveZIPCandidate, string, error) {
	budget := &traceArchiveZIPProbeBudget{remaining: traceArchiveZIPProbeTotalBytes, perMember: traceArchiveZIPProbeMemberBytes}
	return selectTraceArchiveZIPMemberWithBudget(ctx, reader, directory, explicit, budget)
}

func selectTraceArchiveZIPMemberWithBudget(ctx context.Context, reader *zip.Reader, directory traceArchiveZIPDirectory, explicit string, budget *traceArchiveZIPProbeBudget) (traceArchiveZIPCandidate, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if explicit != "" {
		if _, err := validateExplicitTraceArchiveMember(explicit); err != nil {
			return traceArchiveZIPCandidate{}, "", err
		}
	}
	// Audit the entire namespace before probing, even for explicit selection.
	regular, err := traceArchiveZIPRegularMembers(ctx, reader, directory)
	if err != nil {
		return traceArchiveZIPCandidate{}, "", err
	}
	var candidates []traceArchiveZIPCandidate
	for _, member := range regular {
		if err := ctx.Err(); err != nil {
			return traceArchiveZIPCandidate{}, "", err
		}
		if explicit != "" && explicit != member.name {
			continue
		}
		ext := strings.ToLower(path.Ext(member.name))
		known := ext == ".sys" || ext == ".htrace"
		if !known {
			known, err = probeTraceArchiveZIPLeaf(ctx, member.file, budget)
			if err != nil {
				return traceArchiveZIPCandidate{}, "", err
			}
		}
		if known {
			candidates = append(candidates, member)
		}
	}
	if err := ctx.Err(); err != nil {
		return traceArchiveZIPCandidate{}, "", err
	}
	if explicit != "" {
		if len(candidates) == 1 {
			return candidates[0], "explicit_member", nil
		}
		return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeExplicitMember, explicit, errors.New("explicit member is absent or is not a regular .sys/.htrace or recognized binary trace member"))
	}
	switch len(candidates) {
	case 0:
		return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeNoCandidate, "", errors.New("ZIP contains no regular .sys/.htrace or recognized binary trace member"))
	case 1:
		return candidates[0], "unique_candidate", nil
	default:
		return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeMultipleCandidates, "", fmt.Errorf(
			"ZIP contains %d trace candidates (%s); select one with --archive-member", len(candidates), traceArchiveCandidateSummary(candidates)))
	}
}

func traceArchiveZIPRegularMembers(ctx context.Context, reader *zip.Reader, directory traceArchiveZIPDirectory) ([]traceArchiveZIPCandidate, error) {
	if reader == nil {
		return nil, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", errors.New("missing ZIP reader"))
	}
	if uint64(len(reader.File)) != directory.entries || len(reader.File) > traceArchiveZIPMaxEntries {
		return nil, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", fmt.Errorf("ZIP central-directory count mismatch/limit: parsed=%d declared=%d", len(reader.File), directory.entries))
	}
	names := make(map[string]struct{}, len(reader.File))
	var regular []traceArchiveZIPCandidate
	for _, file := range reader.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name, isDir, err := validateTraceArchiveZIPName(file)
		if err != nil {
			return nil, err
		}
		if _, duplicate := names[name]; duplicate {
			return nil, traceArchiveFailure(traceArchiveCodeDuplicateMember, name, errors.New("canonical member name appears more than once"))
		}
		names[name] = struct{}{}
		if file.Flags&1 != 0 {
			return nil, traceArchiveFailure(traceArchiveCodeEncryptedMember, name, errors.New("encrypted ZIP members are unsupported"))
		}
		mode := file.Mode()
		if isDir {
			if !mode.IsDir() && mode.Type() != 0 {
				return nil, traceArchiveFailure(traceArchiveCodeSpecialMember, name, fmt.Errorf("directory member has special mode %s", mode))
			}
			continue
		}
		if mode.Type() != 0 {
			return nil, traceArchiveFailure(traceArchiveCodeSpecialMember, name, fmt.Errorf("member mode %s is not regular", mode))
		}
		regular = append(regular, traceArchiveZIPCandidate{file: file, name: name})
	}
	return regular, nil
}

func traceArchiveCandidateSummary(candidates []traceArchiveZIPCandidate) string {
	const maxShown = 8
	shown := len(candidates)
	if shown > maxShown {
		shown = maxShown
	}
	parts := make([]string, 0, shown+1)
	for index := 0; index < shown; index++ {
		parts = append(parts, strconv.Quote(boundedTraceProviderErrorText(candidates[index].name, 160)))
	}
	if omitted := len(candidates) - shown; omitted > 0 {
		parts = append(parts, fmt.Sprintf("... %d omitted", omitted))
	}
	return strings.Join(parts, ", ")
}
