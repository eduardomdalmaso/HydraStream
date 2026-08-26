package domain

import "errors"

var (
	ErrStreamNotFound   = errors.New("stream not found")
	ErrInvalidStream    = errors.New("invalid stream data")
	ErrConsumerNotFound = errors.New("consumer not found")
)
