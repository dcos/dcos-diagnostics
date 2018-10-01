package units

import (
	"fmt"
	"io"
)

// ReadJournalOutputSince returns error since windows does not support journal
func ReadJournalOutputSince(unit, sinceString string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("there is no journal on Windows")
}
