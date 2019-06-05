package units

import (
	"context"
	"errors"
	"io"
	"time"
)

// ReadJournalOutputSince returns error since windows does not support journal
func ReadJournalOutputSince(ctx context.Context, unit string, duration time.Duration) (io.ReadCloser, error) {
	return nil, errors.New("there is no journal on Windows")
}
