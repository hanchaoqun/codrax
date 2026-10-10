package loginput

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// Query always revalidates complete selected sources before publishing rows.
// A failed source contributes neither provisional rows nor provisional counts;
// healthy sources remain available and Complete becomes false.
func (c *Catalog) Query(ctx context.Context, query Query) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := Result{Records: []Record{}, Sources: []Source{}, Complete: true, Offset: query.Offset, Limit: query.Limit}
	if c == nil {
		return result, fmt.Errorf("no attached log catalog")
	}
	if err := validateQuery(query, c); err != nil {
		return result, err
	}
	if result.Limit == 0 {
		result.Limit = 100
	}
	usedBytes := 0
	for _, original := range c.sources {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if len(query.SourceIDs) > 0 && !contains(query.SourceIDs, original.summary.ID) {
			continue
		}
		if !original.summary.Complete {
			summary := original.summary
			summary.Aliases = append([]string(nil), summary.Aliases...)
			result.Sources = append(result.Sources, summary)
			result.SourceErrors = append(result.SourceErrors, SourceError{original.summary.ID, original.summary.Error})
			result.Complete = false
			continue
		}
		local := original
		local.summary.Aliases = append([]string(nil), original.summary.Aliases...)
		var matched int64
		var records []Record
		localBytes, byteLimit := 0, false
		err := scanSource(ctx, &local, c.opts, func(record Record) {
			if !matchesRecord(record, query) {
				return
			}
			index := result.Matched + matched
			matched++
			if index < query.Offset || len(result.Records)+len(records) >= result.Limit || result.ResultByteLimitReached || byteLimit {
				return
			}
			encoded, _ := json.Marshal(record)
			if len(encoded) > c.opts.MaxResultBytes-usedBytes-localBytes {
				byteLimit = true
				return
			}
			records = append(records, record)
			localBytes += len(encoded)
		}, true)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, ctxErr
		}
		if err != nil {
			local.summary.Complete = false
			local.summary.Error = err.Error()
			// Counts from a changed/corrupt source are not accepted evidence.
			local.summary.Records, local.summary.ParsedRecords, local.summary.UnknownRecords, local.summary.MalformedRecords, local.summary.OrphanContinuations = 0, 0, 0, 0, 0
			result.SourceErrors = append(result.SourceErrors, SourceError{original.summary.ID, err.Error()})
			result.Complete = false
		} else {
			result.Matched += matched
			result.Records = append(result.Records, records...)
			usedBytes += localBytes
			result.ResultByteLimitReached = result.ResultByteLimitReached || byteLimit
		}
		result.Sources = append(result.Sources, local.summary)
	}
	// Earlier members can change while a later member is being scanned. Recheck
	// all accepted physical bindings at the publication boundary as well; never
	// publish a mixed/stale result merely because each earlier scan once passed.
	for _, source := range result.Sources {
		if !source.Complete || source.Path == "" {
			continue
		}
		for _, prepared := range c.sources {
			if prepared.summary.ID != source.ID {
				continue
			}
			for _, path := range append([]string{source.Path}, source.Aliases...) {
				if err := ctx.Err(); err != nil {
					return Result{}, err
				}
				bound, err := filegeneration.FromPath(path)
				if err != nil {
					return Result{}, fmt.Errorf("log source binding unavailable at publication: %w", err)
				}
				if !prepared.identity.SameVersion(bound) {
					return Result{}, fmt.Errorf("log source %s changed before result publication", source.ID)
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	result.Returned = len(result.Records)
	result.Omitted = result.Matched - int64(result.Returned)
	return result, nil
}

func validateQuery(query Query, c *Catalog) error {
	if query.Offset < 0 || query.Limit < 0 || query.Limit > 1000 {
		return fmt.Errorf("offset must be nonnegative and limit must be between 0 and 1000 (0 defaults to 100)")
	}
	if query.FirstLine < 0 || query.LastLine < 0 || (query.LastLine > 0 && query.FirstLine > query.LastLine) {
		return fmt.Errorf("physical line bounds must be nonnegative, inclusive, and ordered")
	}
	if query.PID != nil && *query.PID < 0 || query.TID != nil && *query.TID < 0 {
		return fmt.Errorf("process and thread IDs must be nonnegative")
	}
	for _, kind := range query.Kinds {
		if kind != KindHilog && kind != KindKmsg && kind != KindText {
			return fmt.Errorf("unknown log kind %q", kind)
		}
	}
	for _, id := range query.SourceIDs {
		found := false
		for _, source := range c.sources {
			if source.summary.ID == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown attached log source ID %q", id)
		}
	}
	return nil
}

func matchesRecord(record Record, query Query) bool {
	if len(query.Kinds) > 0 && !contains(query.Kinds, record.Kind) {
		return false
	}
	if query.PID != nil && (record.PID == nil || *record.PID != *query.PID) {
		return false
	}
	if query.TID != nil && (record.TID == nil || *record.TID != *query.TID) {
		return false
	}
	if query.FirstLine > 0 && record.LastLine < query.FirstLine {
		return false
	}
	if query.LastLine > 0 && record.FirstLine > query.LastLine {
		return false
	}
	return query.Contains == "" || bytes.Contains(record.RawBytes, []byte(query.Contains))
}
