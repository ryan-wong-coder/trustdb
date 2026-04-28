package trusterr

import (
	"errors"
	"fmt"
)

type Code string

const (
	CodeInvalidArgument    Code = "INVALID_ARGUMENT"
	CodeFailedPrecondition Code = "FAILED_PRECONDITION"
	CodeAlreadyExists      Code = "ALREADY_EXISTS"
	CodeNotFound           Code = "NOT_FOUND"
	CodeResourceExhausted  Code = "RESOURCE_EXHAUSTED"
	CodeDeadlineExceeded   Code = "DEADLINE_EXCEEDED"
	CodeDataLoss           Code = "DATA_LOSS"
	CodeInternal           Code = "INTERNAL"
)

type Error struct {
	Code    Code
	Message string
	Err     error
}

func New(code Code, message string) error {
	return Error{Code: code, Message: message}
}

func Wrap(code Code, message string, err error) error {
	if err == nil {
		return New(code, message)
	}
	return Error{Code: code, Message: message, Err: err}
}

func (e Error) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e Error) Unwrap() error {
	return e.Err
}

func CodeOf(err error) Code {
	var coded Error
	if errors.As(err, &coded) {
		return coded.Code
	}
	return CodeInternal
}

func ExitCode(err error) int {
	switch CodeOf(err) {
	case CodeInvalidArgument:
		return 2
	case CodeAlreadyExists:
		return 3
	case CodeFailedPrecondition:
		return 4
	case CodeNotFound:
		return 5
	case CodeResourceExhausted:
		return 7
	case CodeDeadlineExceeded:
		return 8
	case CodeDataLoss:
		return 6
	default:
		return 1
	}
}
