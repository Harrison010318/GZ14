package scene

import "errors"

var (
	ErrEntityNotFound = errors.New("entity not found")
	ErrMoveTooFast    = errors.New("move too fast")
	ErrOutOfBounds    = errors.New("out of bounds")
)
