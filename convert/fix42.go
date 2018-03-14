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
)

func FIX42MarketDataFullRefreshFromBookSnapshot(mdReqID string, snapshot *bitfinex.BookUpdateSnapshot) *fix42mdsfr.MarketDataSnapshotFullRefresh {
	if len(snapshot.Snapshot) <= 0 {
		return nil
	}
	first := snapshot.Snapshot[0]
	message := fix42mdsfr.New(field.NewSymbol(defixifySymbol(first.Symbol)))
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

func defixifySymbol(sym string) string {
	/*
		if sym[0] == 't' && len(sym) > 1 {
			return sym[1:]
		}*/
	return sym
}

func FIX42ExecutionReport(symbol, orderID, account string, execType enum.ExecType, side enum.Side, qty, cumQty, avgPx float64, ordStatus enum.OrdStatus, text string) fix42er.ExecutionReport {
	uid, err := uuid.NewV4()
	execID := ""
	if err == nil {
		execID = uid.String()
	}
	// total order qty
	amt := decimal.NewFromFloat(qty)

	// total executed so far
	cumAmt := decimal.NewFromFloat(cumQty)

	// remaining to be executed
	remaining := amt.Sub(cumAmt)

	e := fix42er.New(
		field.NewOrderID(orderID),
		field.NewExecID(execID),
		field.NewExecTransType(enum.ExecTransType_STATUS),
		field.NewExecType(execType),
		field.NewOrdStatus(ordStatus),
		field.NewSymbol(defixifySymbol(symbol)),
		field.NewSide(side),
		field.NewLeavesQty(remaining, 4), // qty
		field.NewCumQty(cumAmt, 4),
		AvgPxToFIX(avgPx),
	)
	e.SetAccount(account)
	if text != "" {
		e.SetText(text)
	}
	return e
}

// used for oc-req notifications where only a cancel's CID is provided
func FIX42ExecutionReportFromCancelWithDetails(c *bitfinex.OrderCancel, account string, execType enum.ExecType, cumQty float64, ordStatus enum.OrdStatus, text, symbol, orderID string, side enum.Side, qty, avgPx float64) fix42er.ExecutionReport {
	e := FIX42ExecutionReport(symbol, orderID, account, execType, side, qty, cumQty, avgPx, ordStatus, text)
	// TODO cxl details?
	return e
}

func FIX42ExecutionReportFromOrder(o *bitfinex.Order, account string, execType enum.ExecType, cumQty float64, ordStatus enum.OrdStatus, text string) fix42er.ExecutionReport {
	orderID := strconv.FormatInt(o.ID, 10)
	// total order qty
	fAmt := o.Amount
	if fAmt < 0 {
		fAmt = -fAmt
	}
	e := FIX42ExecutionReport(o.Symbol, orderID, account, execType, SideToFIX(o.Amount), fAmt, cumQty, o.PriceAvg, ordStatus, text)
	switch o.Type {
	case bitfinex.OrderTypeLimit:
		e.SetPrice(decimal.NewFromFloat(o.Price), 4)
	case bitfinex.OrderTypeStopLimit:
		e.SetPrice(decimal.NewFromFloat(o.Price), 4)
		//e.SetStopPx(decimal.NewFromFloat(o.PriceAuxLimit), 4) // ??
	}
	if text != "" {
		e.SetText(text)
	}
	e.SetLastShares(decimal.Zero, 4) // qty
	return e
}

func FIX42ExecutionReportFromTradeExecutionUpdate(t *bitfinex.TradeExecutionUpdate, account, clOrdID string, origQty, totalFillQty, avgFillPx float64) fix42er.ExecutionReport {
	orderID := strconv.FormatInt(t.OrderID, 10)
	//execID := strconv.FormatInt(t.ID, 10)
	var execType enum.ExecType
	var ordStatus enum.OrdStatus
	if totalFillQty >= origQty {
		execType = enum.ExecType_FILL
		ordStatus = enum.OrdStatus_FILLED
	} else {
		execType = enum.ExecType_PARTIAL_FILL
		ordStatus = enum.OrdStatus_PARTIALLY_FILLED
	}
	execAmt := t.ExecAmount
	if execAmt < 0 {
		execAmt = -execAmt
	}
	totalQty := decimal.NewFromFloat(origQty)
	thisQty := decimal.NewFromFloat(execAmt)
	e := FIX42ExecutionReport(t.Pair, orderID, account, execType, SideToFIX(t.ExecAmount), execAmt, totalFillQty, avgFillPx, ordStatus, "")
	e.SetOrderQty(totalQty, 4)  // qty
	e.SetLastShares(thisQty, 4) // qty

	return e
}

func FIX42OrderCancelRejectFromCancel(o *bitfinex.OrderCancel, account, orderID, clOrdID string) ocj.OrderCancelReject {
	r := ocj.New(
		field.NewOrderID(orderID),
		field.NewClOrdID(clOrdID),
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
