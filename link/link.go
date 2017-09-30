// Package link connects the two parties of a gateway.
package link

import (
	"github.com/bitfinexcom/bfxfixgw/fix"
	"github.com/bitfinexcom/bfxfixgw/log"

	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
)

type Link struct {
	fix *fix.FIX

	logger *zap.Logger
}

func NewLink(s *quickfix.Settings) (*Link, error) {
	f, err := fix.NewFIX(s)
	if err != nil {
		return nil, err
	}

	//var sID *quickfix.SessionID
	//for s, _ := range s.SessionSettings() {
	//// We should only have one session at this point otherwise we're just going to use
	//// the last one.
	//sID = &s
	//}

	l := &Link{logger: log.Logger, fix: f}

	return l, nil
}

func (l *Link) Establish() error {
	err := l.fix.Up()
	if err != nil {
		return err
	}

	return nil
}

func (l *Link) Close() {
	l.fix.Down()
}
