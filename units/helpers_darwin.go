package units

import (
	"errors"
	"io"
)

// ReadJournalOutputSince returns error since darwin does not support journal
func ReadJournalOutputSince(unit, sinceString string) (io.ReadCloser, error) {
	return nil, errors.New("does not work on darwin")
}
