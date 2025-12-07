// Package docker provides sentinel errors for the docker tool package.
package docker

import "errors"

var (
	// ErrEmptyCommand is returned when a docker command is empty.
	ErrEmptyCommand = errors.New("tool/docker: empty command")
)
