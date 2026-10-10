package loginput

import (
	"context"
	"fmt"
	"strings"
)

// ReadRecord retrieves one exact native record identity, revalidating the full
// attached source including bytes after that record. It does not accept paths
// or infer an identity from a model-authored line range. Query page byte budgets
// do not prevent retrieval of a valid large record.
func (c *Catalog) ReadRecord(ctx context.Context, id string) (Record, Source, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Record{}, Source{}, err
	}
	if c == nil {
		return Record{}, Source{}, fmt.Errorf("no attached log catalog")
	}
	for _, prepared := range c.sources {
		if !strings.HasPrefix(id, prepared.summary.ID+":L") {
			continue
		}
		if !prepared.summary.Complete {
			return Record{}, Source{}, fmt.Errorf("record source is unavailable: %s", prepared.summary.Error)
		}
		local := prepared
		local.summary.Aliases = append([]string(nil), prepared.summary.Aliases...)
		var found Record
		err := scanSource(ctx, &local, c.opts, func(record Record) {
			if record.ID == id {
				found = record
			}
		}, true)
		if err != nil {
			return Record{}, Source{}, err
		}
		if err := ctx.Err(); err != nil {
			return Record{}, Source{}, err
		}
		if found.ID == "" {
			return Record{}, Source{}, fmt.Errorf("unknown record_ref in attached source")
		}
		return found, local.summary, nil
	}
	return Record{}, Source{}, fmt.Errorf("record_ref does not identify an attached source")
}
