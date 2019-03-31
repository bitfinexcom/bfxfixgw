package main

import (
	fix "github.com/quickfixgo/quickfix"
	"log"
	"os"
)

func loadSettings(file string) *fix.Settings {
	cfg, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	settings, err := fix.ParseSettings(cfg)
	if err != nil {
		log.Fatal(err)
	}
	return settings
}
