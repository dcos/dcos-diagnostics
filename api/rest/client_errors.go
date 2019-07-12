package rest

import (
	"fmt"
)

type DiagnosticsBundleNotFoundError struct {
	id string
}

func (d *DiagnosticsBundleNotFoundError) Error() string {
	return fmt.Sprintf("bundle %s not found", d.id)
}

type DiagnosticsBundleUnreadableError struct {
	id string
}

func (d *DiagnosticsBundleUnreadableError) Error() string {
	return fmt.Sprintf("bundle %s not readable", d.id)
}

type DiagnosticsBundleNotCompletedError struct {
	id string
}

func (d *DiagnosticsBundleNotCompletedError) Error() string {
	return fmt.Sprintf("bundle %s canceled or already deleted", d.id)
}

type DiagnosticsBundleAlreadyExists struct {
	id string
}

func (d *DiagnosticsBundleAlreadyExists) Error() string {
	return fmt.Sprintf("bundle %s already exists", d.id)
}
