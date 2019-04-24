package units

import (
	"context"
	"errors"
	"io"
)

// ReadJournalOutputSince returns error since darwin does not support journal
func ReadJournalOutputSince(ctx context.Context, unit, sinceString string) (io.ReadCloser, error) {
	return nil, errors.New("does not work on darwin")
}
