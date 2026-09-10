package core

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isConnectionRefused(err error) bool {
	// Winsock uses WSAECONNREFUSED, not Go's compatibility ECONNREFUSED value.
	return errors.Is(err, windows.WSAECONNREFUSED)
}
