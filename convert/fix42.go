// Package convert has utils to build FIX4.(2|4) messages to and from bitfinex
// API responses.
package convert

import (
	"github.com/bitfinexcom/bfxfixgw/service/symbol"
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

func FIX42MarketDataFullRefreshFromTradeSnapshot(mdReqID string, snapshot *bitfinex.TradeSnapshot, symbology symbol.Symbology, counterparty string) *fix42mdsfr.MarketDataSnapshotFullRefresh {
	if len(snapshot.Snapshot) <= 0 {
		return nil
	}
	first := snapshot.Snapshot[0]
	sym, err := symbology.FromBitfinex(first.Pair, counterparty)
	if err != nil {
		sym = first.Pair
	}
	message := fix42mdsfr.New(field.NewSymbol(sym))
	message.SetMDReqID(mdReqID)
	message.SetSymbol(first.Pair)
	message.SetSecurityID(first.Pair)
	message.SetIDSource(enum.IDSource_EXCHANGE_SYMBOL)
	// MDStreamID?
	group := fix42mdsfr.NewNoMDEntriesRepeatingGroup()
	for _, update := range snapshot.Snapshot {
		entry := group.Add()
		entry.SetMDEntryType(enum.MDEntryType_TRADE)
		entry.SetMDEntryPx(decimal.NewFromFloat(update.Price), 4)
		amt := update.Amount
		if amt < 0 {
			amt = -amt
		}
		entry.SetMDEntrySize(decimal.NewFromFloat(amt), 4)
	}
	message.SetNoMDEntries(group)
	return &message
}

func FIX42MarketDataFullRefreshFromBookSnapshot(mdReqID string, snapshot *bitfinex.BookUpdateSnapshot, symbology symbol.Symbology, counterparty string) *fix42mdsfr.MarketDataSnapshotFullRefresh {
	if len(snapshot.Snapshot) <= 0 {
		return nil
	}
	first := snapshot.Snapshot[0]
	sym, err := symbology.FromBitfinex(first.Symbol, counterparty)
	if err != nil {
		sym = first.Symbol
	}
	message := fix42mdsfr.New(field.NewSymbol(sym))
	message.SetMDReqID(mdReqID)
	message.SetSymbol(first.Symbol)
	message.SetSecurityID(first.Symbol)
	message.SetIDSource(enum.IDSource_EXCHANGE_SYMBOL)
	// MDStreamID?
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
		amt := update.Amount
		if amt < 0 {
			amt = -amt
		}
		entry.SetMDEntrySize(decimal.NewFromFloat(amt), 4)
	}
	message.SetNoMDEntries(group)
	return &message
}

func FIX42MarketDataIncrementalRefreshFromTrade(mdReqID string, trade *bitfinex.Trade, symbology symbol.Symbology, counterparty string) *fix42mdir.MarketDataIncrementalRefresh {
	symbol, err := symbology.FromBitfinex(trade.Pair, counterparty)
	if err != nil {
		symbol = trade.Pair
	}

	message := fix42mdir.New()
	message.SetMDReqID(mdReqID)
	// MDStreamID?
	group := fix42mdir.NewNoMDEntriesRepeatingGroup()
	entry := group.Add()
	entry.SetMDEntryType(enum.MDEntryType_TRADE)
	entry.SetMDUpdateAction(enum.MDUpdateAction_NEW)
	entry.SetMDEntryPx(decimal.NewFromFloat(trade.Price), 4)
	entry.SetSecurityID(symbol)
	entry.SetIDSource(enum.IDSource_EXCHANGE_SYMBOL)
	amt := trade.Amount
	if amt < 0 {
		amt = -amt
	}
	entry.SetMDEntrySize(decimal.NewFromFloat(amt), 4)
	entry.SetSymbol(symbol)
	message.SetNoMDEntries(group)
	return &message
}

func FIX42MarketDataIncrementalRefreshFromBookUpdate(mdReqID string, update *bitfinex.BookUpdate, symbology symbol.Symbology, counterparty string) *fix42mdir.MarketDataIncrementalRefresh {
	symbol, err := symbology.FromBitfinex(update.Symbol, counterparty)
	if err != nil {
		symbol = update.Symbol
	}

	message := fix42mdir.New()
	message.SetMDReqID(mdReqID)
	// MDStreamID?
	group := fix42mdir.NewNoMDEntriesRepeatingGroup()
	entry := group.Add()
	var t enum.MDEntryType
	switch update.Side {
	case bitfinex.Bid:
		t = enum.MDEntryType_BID
	case bitfinex.Ask:
		t = enum.MDEntryType_OFFER
	}
	action := BookActionToFIX(update.Action)
	entry.SetMDEntryType(t)
	entry.SetMDUpdateAction(action)
	entry.SetMDEntryPx(decimal.NewFromFloat(update.Price), 4)
	entry.SetSecurityID(symbol)
	entry.SetIDSource(enum.IDSource_EXCHANGE_SYMBOL)
	amt := update.Amount
	if amt < 0 {
		amt = -amt
	}
	if action != enum.MDUpdateAction_DELETE {
		entry.SetMDEntrySize(decimal.NewFromFloat(amt), 4)
	}
	entry.SetSymbol(symbol)
	message.SetNoMDEntries(group)
	return &message
}

func FIX42ExecutionReport(symbol, clOrdID, orderID, account string, execType enum.ExecType, side enum.Side, origQty, thisQty, cumQty, avgPx float64, ordStatus enum.OrdStatus, ordType enum.OrdType, text string, symbology symbol.Symbology, counterparty string) fix42er.ExecutionReport {
	uid, err := uuid.NewV4()
	execID := ""
	if err == nil {
		execID = uid.String()
	}
	// total order qty
	amt := decimal.NewFromFloat(origQty)

	// total executed so far
	cumAmt := decimal.NewFromFloat(cumQty)

	// remaining to be executed
	remaining := amt.Sub(cumAmt)
	switch ordStatus {
	case enum.OrdStatus_CANCELED:
		fallthrough
	case enum.OrdStatus_DONE_FOR_DAY:
		fallthrough
	case enum.OrdStatus_EXPIRED:
		fallthrough
	case enum.OrdStatus_REPLACED:
		fallthrough
	case enum.OrdStatus_STOPPED:
		fallthrough
	case enum.OrdStatus_SUSPENDED:
		remaining = decimal.Zero
	}

	// this execution
	lastShares := decimal.NewFromFloat(thisQty)

	sym, err := symbology.FromBitfinex(symbol, counterparty)
	if err != nil {
		sym = symbol
	}

	e := fix42er.New(
		field.NewOrderID(orderID),
		field.NewExecID(execID),
		field.NewExecTransType(enum.ExecTransType_STATUS),
		field.NewExecType(execType),
		field.NewOrdStatus(ordStatus),
		field.NewSymbol(sym),
		field.NewSide(side),
		field.NewLeavesQty(remaining, 4), // qty
		field.NewCumQty(cumAmt, 4),
		AvgPxToFIX(avgPx),
	)
	e.SetAccount(account)
	if lastShares.Cmp(decimal.Zero) != 0 {
		e.SetLastShares(lastShares, 4)
	}
	e.SetOrderQty(amt, 4)
	if text != "" {
		e.SetText(text)
	}
	e.SetOrdType(ordType)
	e.SetClOrdID(clOrdID)
	return e
}

func FIX42ExecutionReportFromOrder(o *bitfinex.Order, account string, execType enum.ExecType, cumQty float64, ordStatus enum.OrdStatus, text string, symbology symbol.Symbology, counterparty string) fix42er.ExecutionReport {
	orderID := strconv.FormatInt(o.ID, 10)
	// total order qty
	fAmt := o.Amount
	if fAmt < 0 {
		fAmt = -fAmt
	}
	e := FIX42ExecutionReport(o.Symbol, strconv.FormatInt(o.CID, 10), orderID, account, execType, SideToFIX(o.Amount), fAmt, 0.0, cumQty, o.PriceAvg, ordStatus, OrdTypeToFIX(o.Type), text, symbology, counterparty)
	switch o.Type {
	case bitfinex.OrderTypeLimit:
		fallthrough
	case bitfinex.OrderTypeExchangeLimit:
		e.SetPrice(decimal.NewFromFloat(o.Price), 4)
	case bitfinex.OrderTypeStopLimit:
		e.SetPrice(decimal.NewFromFloat(o.Price), 4)
		//e.SetStopPx(decimal.NewFromFloat(o.PriceAuxLimit), 4) // ??
	case bitfinex.OrderTypeStop:
		fallthrough
	case bitfinex.OrderTypeExchangeStop:
		// TODO
	case bitfinex.OrderTypeTrailingStop:
		fallthrough
	case bitfinex.OrderTypeExchangeTrailingStop:
		// TODO
	case bitfinex.OrderTypeFOK:
		fallthrough
	case bitfinex.OrderTypeExchangeFOK:
		// TODO
	}
	// TODO order options?
	if text != "" {
		e.SetText(text)
	}
	e.SetLastShares(decimal.Zero, 4) // qty
	return e
}

func FIX42ExecutionReportFromTradeExecutionUpdate(t *bitfinex.TradeExecutionUpdate, account, clOrdID string, origQty, totalFillQty, origPx, avgFillPx float64, symbology symbol.Symbology, counterparty string) fix42er.ExecutionReport {
	orderID := strconv.FormatInt(t.OrderID, 10)
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
	er := FIX42ExecutionReport(t.Pair, clOrdID, orderID, account, execType, SideToFIX(t.ExecAmount), origQty, execAmt, totalFillQty, avgFillPx, ordStatus, OrdTypeToFIX(t.OrderType), "", symbology, counterparty)
	f := t.Fee
	if f < 0 {
		f = -f
	}
	// TODO order type specific fields?
	fee := decimal.NewFromFloat(f)
	er.SetCommission(fee, 4)
	er.SetCommType(enum.CommType_ABSOLUTE)
	er.SetLastPx(decimal.NewFromFloat(t.ExecPrice), 4)
	if origPx > 0 {
		er.SetPrice(decimal.NewFromFloat(origPx), 4)
	}
	return er
}

func rejectReasonFromText(text string) enum.CxlRejReason {
	switch text {
	case "Order not found.":
		return enum.CxlRejReason_UNKNOWN_ORDER
	}
	return enum.CxlRejReason_OTHER
}

func FIX42OrderCancelRejectFromCancel(o *bitfinex.OrderCancel, account, orderID, origClOrdID, cxlClOrdID, text string) ocj.OrderCancelReject {
	r := ocj.New(
		field.NewOrderID(orderID),
		field.NewClOrdID(cxlClOrdID),
		field.NewOrigClOrdID(origClOrdID),
		field.NewOrdStatus(enum.OrdStatus_REJECTED),
		field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
	)
	r.SetCxlRejReason(rejectReasonFromText(text))
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
