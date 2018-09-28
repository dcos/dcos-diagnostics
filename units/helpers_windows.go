package units

import (
	"fmt"
	"io"
)

func ReadJournalOutputSince(unit, sinceString string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("there is no journal on Windows")
}
