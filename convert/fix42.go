// Package convert has utils to build FIX4.(2|4) messages to and from bitfinex
// API responses.
package convert

import (
	//"errors"
	"strconv"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
	uuid "github.com/satori/go.uuid"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/shopspring/decimal"

	fix42er "github.com/quickfixgo/fix42/executionreport"
	fix42mdir "github.com/quickfixgo/fix42/marketdataincrementalrefresh"
	fix42mdsfr "github.com/quickfixgo/fix42/marketdatasnapshotfullrefresh"
	ocj "github.com/quickfixgo/fix42/ordercancelreject"
	//fix42nos "github.com/quickfixgo/quickfix/fix42/newordersingle"
	"github.com/bitfinexcom/bfxfixgw/log"
	"go.uber.org/zap"
)

func FIX42MarketDataFullRefreshFromBookSnapshot(mdReqID string, snapshot *bitfinex.BookUpdateSnapshot) *fix42mdsfr.MarketDataSnapshotFullRefresh {
	if len(snapshot.Snapshot) <= 0 {
		return nil
	}
	first := snapshot.Snapshot[0]
	message := fix42mdsfr.New(field.NewSymbol(first.Symbol))
	message.SetMDReqID(mdReqID)
	// TODO securityID?
	// TODO securityIDSource?
	group := fix42mdsfr.NewNoMDEntriesRepeatingGroup()
	for _, update := range snapshot.Snapshot {
		entry := group.Add()
		var t enum.MDEntryType
		switch update.Side {
		case bitfinex.Bid:
			t = enum.MDEntryType_BID
		case bitfinex.Ask:
			t = enum.MDEntryType_OFFER
		}
		entry.SetMDEntryType(t)
		entry.SetMDEntryPx(decimal.NewFromFloat(update.Price), 4)
		entry.SetMDEntrySize(decimal.NewFromFloat(update.Amount), 4)
	}
	message.SetNoMDEntries(group)
	return &message
}

func FIX42MarketDataIncrementalRefreshFromBookUpdate(mdReqID string, update *bitfinex.BookUpdate) *fix42mdir.MarketDataIncrementalRefresh {
	message := fix42mdir.New()
	message.SetMDReqID(mdReqID)
	// TODO securityID?
	// TODO securityIDSource?
	// TODO symbol?
	group := fix42mdir.NewNoMDEntriesRepeatingGroup()
	entry := group.Add()
	var t enum.MDEntryType
	switch update.Side {
	case bitfinex.Bid:
		t = enum.MDEntryType_BID
	case bitfinex.Ask:
		t = enum.MDEntryType_OFFER
	}
	entry.SetMDEntryType(t)
	entry.SetMDEntryPx(decimal.NewFromFloat(update.Price), 4)
	entry.SetMDEntrySize(decimal.NewFromFloat(update.Amount), 4)
	message.SetNoMDEntries(group)
	return &message
}

func FIX42ExecutionReportFromOrder(o *bitfinex.Order, account string, execType enum.ExecType) fix42er.ExecutionReport {
	uid, err := uuid.NewV4()
	execID := ""
	if err != nil {
		execID = uid.String()
	}
	orderID := strconv.FormatInt(o.ID, 10)
	log.Logger.Info("creating execution report mapping", zap.String("orderID", orderID), zap.String("execType", string(execType)), zap.String("execID", execID))
	e := fix42er.New(
		field.NewOrderID(orderID),
		field.NewExecID(execID),
		field.NewExecTransType(enum.ExecTransType_STATUS),
		field.NewExecType(execType),

		OrdStatusFromOrder(o),
		field.NewSymbol(o.Symbol),
		SideFromOrder(o),
		LeavesQtyFromOrder(o),
		CumQtyFromOrder(o),
		AvgPxFromOrder(o),
	)
	e.SetAccount(account)

	return e
}

func FIX42OrderCancelRejectFromCancel(o *bitfinex.OrderCancel, account string) ocj.OrderCancelReject {
	// TODO add cache to attempt lookup order IDs
	r := ocj.New(
		field.NewOrderID("NONE"),
		field.NewClOrdID("NONE"), // XXX: This should be the actual ClOrdID which we don't have in this context.
		field.NewOrigClOrdID(strconv.FormatInt(o.CID, 10)),
		field.NewOrdStatus(enum.OrdStatus_REJECTED),
		field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
	)
	r.SetCxlRejReason(enum.CxlRejReason_UNKNOWN_ORDER)
	r.SetAccount(account)
	return r
}

func FIX42NoMDEntriesRepeatingGroupFromTradeTicker(data []float64) fix42mdsfr.NoMDEntriesRepeatingGroup {
	mdEntriesGroup := fix42mdsfr.NewNoMDEntriesRepeatingGroup()

	mde := mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_BID)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[0]), 2)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[1]), 3)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_OFFER)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[2]), 2)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[3]), 3)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADE)
	mde.SetMDEntryPx(decimal.NewFromFloat(data[6]), 2)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADE_VOLUME)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[7]), 8)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADING_SESSION_HIGH_PRICE)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[8]), 2)

	mde = mdEntriesGroup.Add()
	mde.SetMDEntryType(enum.MDEntryType_TRADING_SESSION_LOW_PRICE)
	mde.SetMDEntrySize(decimal.NewFromFloat(data[9]), 2)

	return mdEntriesGroup
}
