package units

import (
	"io"
	"time"

	"github.com/dcos/dcos-log/dcos-log/journal/reader"
	"github.com/sirupsen/logrus"
)

const (
	// _SYSTEMD_UNIT and UNIT are custom fields used by systemd to mark logs by the systemd unit itself and
	// also by other related components. When dcos-diagnostics reads log entries it needs to filter both entries.
	systemdUnitProperty = "_SYSTEMD_UNIT"
	unitProperty        = "UNIT"
)

// ReadJournalOutputSince returns logs since given duration from journal
func ReadJournalOutputSince(unit, sinceString string) (io.ReadCloser, error) {
	matches := DefaultSystemdMatches(unit)
	duration, err := time.ParseDuration(sinceString)
	if err != nil {
		logrus.Errorf("Error parsing %s. Defaulting to 24 hours", sinceString)
		duration = time.Hour * 24
	}
	format := reader.NewEntryFormatter("text/plain", false)
	return reader.NewReader(format, reader.OptionMatchOR(matches), reader.OptionSince(duration))
}

// DefaultSystemdMatches returns default reader.JournalEntryMatch for a given systemd unit.
func DefaultSystemdMatches(unit string) []reader.JournalEntryMatch {
	return []reader.JournalEntryMatch{
		{
			Field: systemdUnitProperty,
			Value: unit,
		},
		{
			Field: unitProperty,
			Value: unit,
		},
	}
}
