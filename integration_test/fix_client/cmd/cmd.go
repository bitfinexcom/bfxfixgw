package cmd

import (
	fix "github.com/quickfixgo/quickfix"
)

type FIXPublisher interface {
	SendFIX(msg fix.Messagable)
}

// Cmd runs commands.
type Cmd interface {
	Execute(keyboard <-chan string, publisher FIXPublisher)
	Handle(msg *fix.Message)
}
