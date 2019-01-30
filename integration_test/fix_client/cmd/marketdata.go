package cmd

import (
	"fmt"
	"log"
	"strconv"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	fix "github.com/quickfixgo/quickfix"

	mdr "github.com/quickfixgo/fix42/marketdatarequest"
	//mdir "github.com/quickfixgo/fix42/marketdataincrementalrefresh"
	"github.com/quickfixgo/tag"
)

//FixPricePrecision is the FIX tag for price precision
const FixPricePrecision fix.Tag = 20003

func newMdRequest(reqID, symbol string, depth int, precision string) *mdr.MarketDataRequest {
	mdreq := mdr.New(field.NewMDReqID(reqID), field.NewSubscriptionRequestType(enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES), field.NewMarketDepth(depth))
	if precision != "" {
		mdreq.SetString(FixPricePrecision, precision)
	}
	nrsg := mdr.NewNoRelatedSymRepeatingGroup()
	nrs := nrsg.Add()
	nrs.Set(field.NewSymbol(symbol))
	mdreq.SetNoRelatedSym(nrsg)
	return &mdreq
}

func buildFixMdRequests(symbols []string, depth int, raw bool, precLevel string) []fix.Messagable {
	reqs := make([]fix.Messagable, 0, len(symbols))
	for _, sym := range symbols {
		reqID := fmt.Sprintf("req-%s", sym)
		precision := ""
		if raw {
			precision = "R0"
		}
		req := newMdRequest(reqID, sym, depth, precision)
		reqs = append(reqs, req)
	}
	return reqs
}

//MarketData is a FIX message builder for Market Data messages
type MarketData struct {
}

//Execute builds Market Data messages
func (m *MarketData) Execute(keyboard <-chan string, publisher FIXPublisher) {
	log.Print("-> Market Data Request")
	log.Print("Enter symbol: ")
	symbol := <-keyboard
	log.Print("Raw? (false for price aggregation)")
	raw, err := strconv.ParseBool(<-keyboard)
	if err != nil {
		log.Printf("raw not bool: %s", err.Error())
		return
	}
	var agg string
	if !raw {
		log.Print("Price aggregation level (P0, P1, etc.)?")
		agg = <-keyboard
	}
	log.Print("Enter depth: ")
	lv := <-keyboard
	depth, err := strconv.Atoi(lv)
	if err != nil {
		log.Printf("depth not int: %s", err.Error())
		return
	}
	reqs := buildFixMdRequests([]string{symbol}, depth, raw, agg)
	for _, req := range reqs {
		publisher.SendFIX(req)
	}
}

//Handle processes Market Data messages
func (m *MarketData) Handle(msg *fix.Message) {
	msgtype, _ := msg.Header.GetString(tag.MsgType)
	if msgtype == "X" || msgtype == "W" {
		log.Printf("[MARKETDATA] %s", msg.String())
	}
}
