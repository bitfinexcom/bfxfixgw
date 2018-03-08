package cmd

import (
	"log"
	"time"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	cxl "github.com/quickfixgo/fix42/ordercancelrequest"
	fix "github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/tag"
)

func buildFixCancel(cancelClOrdID, clOrdID, symbol string, side enum.Side) fix.Messagable {
	return cxl.New(field.NewOrigClOrdID(cancelClOrdID), field.NewClOrdID(clOrdID), field.NewSymbol(symbol), field.NewSide(side), field.NewTransactTime(time.Now()))
}

type Cancel struct {
}

func (c *Cancel) Execute(keyboard <-chan string, publisher FIXPublisher) {
	log.Print("-> Cancel")
	log.Printf("Enter ClOrdID to cancel (integer): ")
	cancelid := <-keyboard
	log.Printf("Enter ClOrdID for this message (integer): ")
	clordid := <-keyboard
	log.Print("Enter symbol: ")
	symbol := <-keyboard
	log.Print("Enter side: ")
	str := <-keyboard
	var side enum.Side
	if str == "buy" {
		side = enum.Side_BUY
	}
	if str == "sell" {
		side = enum.Side_SELL
	}
	cancel := buildFixCancel(cancelid, clordid, symbol, side)
	publisher.SendFIX(cancel)
}

func (c *Cancel) Handle(msg *fix.Message) {
	msgtype, _ := msg.Header.GetString(tag.MsgType)
	if msgtype == "8" {
		//log.Printf("[CANCEL]: %s", msg.String())
	}
}
