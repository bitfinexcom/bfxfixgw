package cmd

import (
	"log"
	"strconv"
	"time"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	nos "github.com/quickfixgo/fix42/newordersingle"
	fix "github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/tag"
)

func buildFixOrder(clordid, symbol string, px, qty float64, side enum.Side, ordType enum.OrdType) fix.Messagable {
	return nos.New(field.NewClOrdID(clordid),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol(symbol), field.NewSide(side),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(ordType))
}

type Order struct {
}

func (o *Order) Execute(keyboard <-chan string, publisher FIXPublisher) {
	log.Print("-> New Order Single")
	log.Printf("Enter ClOrdID (integer): ")
	clordid := <-keyboard
	log.Print("Enter symbol: ")
	symbol := <-keyboard
	log.Print("Enter order type: ")
	str := <-keyboard
	var ordtype enum.OrdType
	if str == "market" {
		ordtype = enum.OrdType_MARKET
	}
	if str == "limit" {
		ordtype = enum.OrdType_LIMIT
	}
	if str == "stop" {
		ordtype = enum.OrdType_STOP
	}
	log.Print("Enter px: ")
	str = <-keyboard
	px, err := strconv.ParseFloat(str, 64)
	if err != nil {
		log.Printf("could not read px: %s", err.Error())
		return
	}
	log.Print("Enter qty: ")
	str = <-keyboard
	qty, err := strconv.ParseFloat(str, 64)
	if err != nil {
		log.Printf("could not read qty: %s", err.Error())
		return
	}
	log.Print("Enter side: ")
	str = <-keyboard
	var side enum.Side
	if str == "buy" {
		side = enum.Side_BUY
	}
	if str == "sell" {
		side = enum.Side_SELL
	}
	nos := buildFixOrder(clordid, symbol, px, qty, side, ordtype)
	publisher.SendFIX(nos)
}

func (o *Order) Handle(msg *fix.Message) {
	msgtype, _ := msg.Header.GetString(tag.MsgType)
	if msgtype == "8" {
		log.Printf("[ORDER]: %s", msg.String())
	}
}
