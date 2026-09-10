//go:build !windows

package core

import (
	"errors"
	"syscall"
)

func isConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
