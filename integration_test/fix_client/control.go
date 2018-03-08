package main

import (
	"bufio"
	"log"
	"os"

	"github.com/bitfinexcom/bfxfixgw/integration_test/fix_client/cmd"

	fix "github.com/quickfixgo/quickfix"
)

type control struct {
	publisher cmd.FIXPublisher
	keyboard  chan string
	cmds      map[string]cmd.Cmd
	current   cmd.Cmd
}

func newControl(publisher cmd.FIXPublisher) *control {
	return &control{
		publisher: publisher,
		keyboard:  make(chan string),
		cmds:      make(map[string]cmd.Cmd),
	}
}

func (c *control) Handle(msg *fix.Message) {
	for _, cmd := range c.cmds {
		cmd.Handle(msg)
	}
}

func (c *control) run() {
	c.keyboard = make(chan string)
	reader := bufio.NewReader(os.Stdin)
	for {
		log.Print("Enter command: ")
		ln, _ := reader.ReadString('\n')
		if ln == "exit" {
			break
		}
		c.keyboard <- ln
	}
	close(c.keyboard) // ?
}

func (c *control) loop() {
	for ln := range c.keyboard {
		for name, cmd := range c.cmds {
			if name == ln {
				cmd.Execute(c.keyboard, c.publisher)
			}
		}
	}
}
