package cmd

import (
	fix "github.com/quickfixgo/quickfix"
)

// FIXPublisher publishes a message to the FIX gateway
type FIXPublisher interface {
	SendFIX(msg fix.Messagable) error
}

// Cmd runs commands.
type Cmd interface {
	Execute(keyboard <-chan string, publisher FIXPublisher) error
	Handle(msg *fix.Message)
}
