package main

import (
	fix "github.com/quickfixgo/quickfix"
	"github.com/shopspring/decimal"
	"log"
)

type entry struct {
	Price, Volume decimal.Decimal
}

type spread struct {
	Bid, Ask, Trade *entry
}

func (s *spread) print() {
	if s.Bid != nil && s.Ask != nil && s.Trade != nil {
		spd := s.Ask.Price.Sub(s.Bid.Price)
		log.Printf("Spread: %s, Bid: %s x %s, Ask: %s x %s, Trade: %s x %s",
			spd,
			s.Bid.Price.String(),
			s.Bid.Volume.String(),
			s.Ask.Price.String(),
			s.Ask.Volume.String(),
			s.Trade.Price.String(),
			s.Trade.Volume.String())
	}
}

const (
	typeBid   string = "0"
	typeAsk   string = "1"
	typeTrade string = "2"
)

func (s *spread) Handle(msg *fix.Message) {
	// parse
	msgType, err := msg.Header.GetString(fix.Tag(35))
	if err != nil {
		log.Print("msg type not found")
		return
	}
	if msgType != "X" {
		log.Printf("msg type not supported: %s", msgType)
		return
	}
	px, err := msg.Body.GetString(fix.Tag(270))
	if err != nil {
		log.Printf("could not get price: %s", err.Error())
		return
	}
	decPx, err2 := decimal.NewFromString(px)
	if err2 != nil {
		log.Printf("could not get price: %s", err2.Error())
		return
	}
	sz, err := msg.Body.GetString(fix.Tag(271))
	if err != nil {
		log.Printf("could not get size: %s", err.Error())
		return
	}
	decSz, err2 := decimal.NewFromString(sz)
	if err2 != nil {
		log.Printf("could not get size: %s", err2.Error())
		return
	}
	act, err := msg.Body.GetString(fix.Tag(279))
	if err != nil {
		log.Printf("could not get update action: %s", err.Error())
		return
	}
	if act != "0" {
		log.Printf("received update without New action: %s", defixify(msg))
	}
	typ, err := msg.Body.GetString(fix.Tag(269))
	if err != nil {
		log.Printf("could not get entry type: %s", err.Error())
		return
	}

	// process
	e := &entry{
		Price:  decPx,
		Volume: decSz,
	}
	if typ == typeBid {
		s.Bid = e
	} else if typ == typeAsk {
		s.Ask = e
	} else if typ == typeTrade {
		s.Trade = e
	}

	s.print()
}
