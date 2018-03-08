package main

import (
	"bufio"
	"log"
	"os"
	"strings"

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

func (c *control) read() {
	reader := bufio.NewReader(os.Stdin)
	for {
		ln, err := reader.ReadString('\n')
		if err != nil {
			close(c.keyboard)
			return
		}
		c.keyboard <- strings.Trim(ln, "\n")
	}
}

func (c *control) run() {
	c.keyboard = make(chan string)
	go c.read()
	for {
		log.Print("Enter command: ")
		for ln := range c.keyboard {
			found := false
			for name, cmd := range c.cmds {
				if name == ln {
					found = true
					cmd.Execute(c.keyboard, c.publisher)
				}
			}
			if !found {
				log.Printf("command not recognized: %s", ln)
			}
			log.Print("Enter command: ")
		}
	}
}
