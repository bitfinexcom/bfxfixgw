package main

import (
	"fmt"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	mdr "github.com/quickfixgo/fix44/marketdatarequest"
	fix "github.com/quickfixgo/quickfix"
	"log"
	"os"
	"strings"
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

func newMdRequest(reqID, symbol string, depth int) *mdr.MarketDataRequest {
	mdreq := mdr.New(field.NewMDReqID(reqID), field.NewSubscriptionRequestType(enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES), field.NewMarketDepth(depth))
	nrsg := mdr.NewNoRelatedSymRepeatingGroup()
	nrs := nrsg.Add()
	nrs.Set(field.NewSymbol(symbol))
	mdreq.SetNoRelatedSym(nrsg)
	return &mdreq
}

func buildFixRequests(symbols []string) []fix.Messagable {
	reqs := make([]fix.Messagable, 0, len(symbols))
	for _, sym := range symbols {
		reqID := fmt.Sprintf("req-%s", sym)
		req := newMdRequest(reqID, sym, 1)
		reqs = append(reqs, req)
	}
	return reqs
}
