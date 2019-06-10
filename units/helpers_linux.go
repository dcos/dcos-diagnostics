package units

import (
	"context"
	goio "io"
	"time"

	"github.com/coreos/go-systemd/sdjournal"

	"github.com/dcos/dcos-diagnostics/io"
)

// ReadJournalOutputSince returns logs since given duration from journal
func ReadJournalOutputSince(ctx context.Context, unit string, duration time.Duration) (goio.ReadCloser, error) {
	return readJournalOutput(ctx, unit, duration, 0)
}

// ReadJournalTail returns numFromTail log lines from the end of the log
func ReadJournalTail(ctx context.Context, unit string, numFromTail uint64) (goio.ReadCloser, error) {
	return readJournalOutput(ctx, unit, 0, numFromTail)
}

func readJournalOutput(ctx context.Context, unit string, d time.Duration, n uint64) (goio.ReadCloser, error) {
	// We need to pass d for the past not future. So it need to negative
	if d > 0 {
		d = -d
	}
	config := sdjournal.JournalReaderConfig{
		Since:       d,
		NumFromTail: n,
		Matches: []sdjournal.Match{
			{Field: sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT, Value: unit},
		},
	}

	src, err := sdjournal.NewJournalReader(config)

	return io.ReadCloserWithContext(ctx, src), err
}
