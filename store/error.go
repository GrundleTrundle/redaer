package store

import (
	"fmt"
)

// Error type.  Whether an error is transient or not is
// significant to us.
type LinkError struct {
	Msg       string
	Transient bool
}

// LinkError is error
func (le *LinkError) Error() string {
	if le.Transient {
		return fmt.Sprintf("TRANSIENT: %s", le.Msg)
	} else {
		return le.Msg
	}
}

func IsTransient(err error) bool {
	le, ok := err.(*LinkError)
	if ok {
		return le.Transient
	}
	return false
}

func MkError(format string, args ...interface{}) error {
	return &LinkError{Msg: fmt.Sprintf(format, args...)}
}

func MkTransientError(format string, args ...interface{}) error {
	return &LinkError{Msg: fmt.Sprintf(format, args...),
		Transient: true}
}
