package units

import (
	"context"
	"errors"
	"io"
	"time"
)

// ReadJournalOutputSince returns error since darwin does not support journal
func ReadJournalOutputSince(ctx context.Context, unit string, duration time.Duration) (io.ReadCloser, error) {
	return nil, errors.New("does not work on darwin")
}
