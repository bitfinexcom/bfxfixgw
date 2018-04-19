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
	"github.com/shopspring/decimal"
)

func buildFixOrder(clordid, symbol string, px, stop, qty float64, side enum.Side, ordType enum.OrdType) *nos.NewOrderSingle {
	ord := nos.New(field.NewClOrdID(clordid),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol(symbol), field.NewSide(side),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(ordType))
	ord.SetOrderQty(decimal.NewFromFloat(qty), 4)
	ord.SetSide(side)
	switch ordType {
	case enum.OrdType_LIMIT:
		ord.SetPrice(decimal.NewFromFloat(px), 4)
	case enum.OrdType_STOP_LIMIT:
		ord.SetPrice(decimal.NewFromFloat(px), 4)
		fallthrough
	case enum.OrdType_STOP:
		ord.SetStopPx(decimal.NewFromFloat(stop), 4)
	}
	return &ord
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
	if str == "stop limit" {
		ordtype = enum.OrdType_STOP_LIMIT
	}
	var err error
	var px, stop float64
	switch ordtype {
	case enum.OrdType_MARKET:
		// no-op
	case enum.OrdType_STOP_LIMIT:
		fallthrough
	case enum.OrdType_LIMIT:
		log.Print("Enter px: ")
		str = <-keyboard
		px, err = strconv.ParseFloat(str, 64)
		if err != nil {
			log.Printf("could not read px: %s", err.Error())
			return
		}
	}
	peg := 0.0
	if ordtype == enum.OrdType_STOP_LIMIT || ordtype == enum.OrdType_STOP {
		log.Print("trailing stop?")
		str = <-keyboard
		if str == "true" || str == "yes" {
			log.Print("Enter stop peg: ")
			str = <-keyboard
			peg, err = strconv.ParseFloat(str, 64)
			if err != nil {
				log.Print("could not parse stop peg")
				return
			}
		} else {
			log.Print("Enter stop px: ")
			str = <-keyboard
			stop, err = strconv.ParseFloat(str, 64)
			if err != nil {
				log.Printf("could not read stop px: %s", err.Error())
				return
			}
		}

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
	nos := buildFixOrder(clordid, symbol, px, stop, qty, side, ordtype)

	log.Print("Options? (hidden, postonly, fok): ")
	str = <-keyboard
	if str == "hidden" {
		nos.SetString(tag.DisplayMethod, string(enum.DisplayMethod_UNDISCLOSED))
	}
	if str == "postonly" {
		nos.SetExecInst(enum.ExecInst_PARTICIPANT_DONT_INITIATE)
	}
	if str == "fok" {
		nos.SetTimeInForce(enum.TimeInForce_FILL_OR_KILL)
	}
	if peg != 0 {
		nos.SetExecInst(enum.ExecInst_PRIMARY_PEG)
		nos.SetPegDifference(decimal.NewFromFloat(peg), 4)
	}

	publisher.SendFIX(nos)
}

func (o *Order) Handle(msg *fix.Message) {
	msgtype, _ := msg.Header.GetString(tag.MsgType)
	if msgtype == "8" {
		log.Printf("[ORDER]: %s", msg.String())
	}
}
