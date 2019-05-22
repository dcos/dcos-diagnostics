package api

import (
	"bytes"
	"encoding/json"
)

type Status int

const (
	Unknown    Status = iota // No information about this bundle
	Started                  // Diagnostics is preparing
	InProgress               // Diagnostics in progress
	Done                     // Diagnostics finished and the file is ready to be downloaded
	Canceled                 // Diagnostics has been cancelled
	Deleted                  // Diagnostics was finished but was deleted
)

func (s Status) String() string {
	return toString[s]
}

var toString = map[Status]string{
	Unknown:    "Unknown",
	Started:    "Started",
	InProgress: "InProgress",
	Done:       "Done",
	Canceled:   "Canceled",
	Deleted:    "Deleted",
}

var toID = map[string]Status{
	"Unknown":    Unknown,
	"Started":    Started,
	"InProgress": InProgress,
	"Done":       Done,
	"Canceled":   Canceled,
	"Deleted":    Deleted,
}

// MarshalJSON marshals the enum as a quoted json string
func (s Status) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(toString[s])
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

// UnmarshalJSON unmashals a quoted json string to the enum value
func (s *Status) UnmarshalJSON(b []byte) error {
	var j string
	err := json.Unmarshal(b, &j)
	if err != nil {
		return err
	}
	// Note that if the string cannot be found then it will be set to the zero value, 'Created' in this case.
	*s = toID[j]
	return nil
}
