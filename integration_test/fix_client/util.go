package main

import (
	"log"
	"os"
	"strings"

	fix "github.com/quickfixgo/quickfix"
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

func defixify(fix *fix.Message) string {
	return strings.Replace(fix.String(), string(0x1), "|", -1)
}
