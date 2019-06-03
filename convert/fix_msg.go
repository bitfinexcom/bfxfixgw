// Package convert has utils to build FIX4.(2|4) messages to and from bitfinex
// API responses.
package convert

import (
	"github.com/quickfixgo/quickfix"
	"strconv"
	"time"

	"github.com/bitfinexcom/bfxfixgw/service/symbol"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
	uuid "github.com/satori/go.uuid"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/shopspring/decimal"

	fix42er "github.com/quickfixgo/fix42/executionreport"
	fix42mdir "github.com/quickfixgo/fix42/marketdataincrementalrefresh"
	fix42mdsfr "github.com/quickfixgo/fix42/marketdatasnapshotfullrefresh"
	ocj42 "github.com/quickfixgo/fix42/ordercancelreject"
	fix44er "github.com/quickfixgo/fix44/executionreport"
	fix44mdir "github.com/quickfixgo/fix44/marketdataincrementalrefresh"
	fix44mdsfr "github.com/quickfixgo/fix44/marketdatasnapshotfullrefresh"
	ocj44 "github.com/quickfixgo/fix44/ordercancelreject"
	fix50er "github.com/quickfixgo/fix50/executionreport"
	fix50mdir "github.com/quickfixgo/fix50/marketdataincrementalrefresh"
	fix50mdsfr "github.com/quickfixgo/fix50/marketdatasnapshotfullrefresh"
	ocj50 "github.com/quickfixgo/fix50/ordercancelreject"
)

//OrderNotFoundText is the text that corresponds to an unknown order
const OrderNotFoundText = "Order not found."

//UnsupportedBeginStringText is the text that corresponds to an unknown beginstring
const UnsupportedBeginStringText = "Unsupported BeginString"

//GenericFix is a simple interface for all generic FIX messages
type GenericFix interface {
	Set(field quickfix.FieldWriter) *quickfix.FieldMap
	SetGroup(field quickfix.FieldGroupWriter) *quickfix.FieldMap
	quickfix.Messagable
}

// FIXMarketDataFullRefreshFromTradeSnapshot generates a market data full refresh
func FIXMarketDataFullRefreshFromTradeSnapshot(beginString, mdReqID string, snapshot *bitfinex.TradeSnapshot, symbology symbol.Symbology, counterparty string) (message GenericFix) {
	if len(snapshot.Snapshot) <= 0 {
		return nil
	}
	first := snapshot.Snapshot[0]
	sym, err := symbology.FromBitfinex(first.Pair, counterparty)
	if err != nil {
		sym = first.Pair
	}
	switch beginString {
	case quickfix.BeginStringFIX42:
		message = fix42mdsfr.New(field.NewSymbol(sym))
	case quickfix.BeginStringFIX44:
		message = fix44mdsfr.New()
		message.Set(field.NewSymbol(sym))
	case quickfix.BeginStringFIXT11:
		message = fix50mdsfr.New()
		message.Set(field.NewSymbol(sym))
	default:
		panic(UnsupportedBeginStringText)
	}
	message.Set(field.NewMDReqID(mdReqID))
	message.Set(field.NewSymbol(sym))
	message.Set(field.NewSecurityID(sym))
	message.Set(field.NewIDSource(enum.IDSource_EXCHANGE_SYMBOL))

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
	message.SetGroup(group)
	return
}

// FIXMarketDataFullRefreshFromBookSnapshot generates a market data full refresh
func FIXMarketDataFullRefreshFromBookSnapshot(beginString, mdReqID string, snapshot *bitfinex.BookUpdateSnapshot, symbology symbol.Symbology, counterparty string) (message GenericFix) {
	if len(snapshot.Snapshot) <= 0 {
		return nil
	}
	first := snapshot.Snapshot[0]
	sym, err := symbology.FromBitfinex(first.Symbol, counterparty)
	if err != nil {
		sym = first.Symbol
	}
	switch beginString {
	case quickfix.BeginStringFIX42:
		message = fix42mdsfr.New(field.NewSymbol(sym))
	case quickfix.BeginStringFIX44:
		message = fix44mdsfr.New()
		message.Set(field.NewSymbol(sym))
	case quickfix.BeginStringFIXT11:
		message = fix50mdsfr.New()
		message.Set(field.NewSymbol(sym))
	default:
		panic(UnsupportedBeginStringText)
	}
	message.Set(field.NewMDReqID(mdReqID))
	message.Set(field.NewSymbol(sym))
	message.Set(field.NewSecurityID(sym))
	message.Set(field.NewIDSource(enum.IDSource_EXCHANGE_SYMBOL))

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
	message.SetGroup(group)
	return
}

// FIXMarketDataIncrementalRefreshFromTrade makes an incremental refresh entry from a trade
func FIXMarketDataIncrementalRefreshFromTrade(beginString, mdReqID string, trade *bitfinex.Trade, symbology symbol.Symbology, counterparty string) (message GenericFix) {
	symbol, err := symbology.FromBitfinex(trade.Pair, counterparty)
	if err != nil {
		symbol = trade.Pair
	}

	switch beginString {
	case quickfix.BeginStringFIX42:
		message = fix42mdir.New()
	case quickfix.BeginStringFIX44:
		message = fix44mdir.New()
	case quickfix.BeginStringFIXT11:
		message = fix50mdir.New()
	default:
		panic(UnsupportedBeginStringText)
	}
	message.Set(field.NewMDReqID(mdReqID))
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
	message.SetGroup(group)
	return
}

// FIXMarketDataIncrementalRefreshFromBookUpdate makes an incremental refresh entry from a book update
func FIXMarketDataIncrementalRefreshFromBookUpdate(beginString, mdReqID string, update *bitfinex.BookUpdate, symbology symbol.Symbology, counterparty string) (message GenericFix) {
	symbol, err := symbology.FromBitfinex(update.Symbol, counterparty)
	if err != nil {
		symbol = update.Symbol
	}

	switch beginString {
	case quickfix.BeginStringFIX42:
		message = fix42mdir.New()
	case quickfix.BeginStringFIX44:
		message = fix44mdir.New()
	case quickfix.BeginStringFIXT11:
		message = fix50mdir.New()
	default:
		panic(UnsupportedBeginStringText)
	}
	message.Set(field.NewMDReqID(mdReqID))
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
	message.SetGroup(group)
	return
}

// FIXExecutionReport generates a FIX execution report from provided order details
func FIXExecutionReport(beginString, symbol, clOrdID, orderID, account string, execType enum.ExecType, side enum.Side, origQty, thisQty, cumQty, px, stop, trail, avgPx float64, ordStatus enum.OrdStatus, ordType enum.OrdType, isMargin bool, tif enum.TimeInForce, exp time.Time, text string, symbology symbol.Symbology, counterparty string, flags int) (e GenericFix) {
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

	switch beginString {
	case quickfix.BeginStringFIX42:
		e = fix42er.New(
			field.NewOrderID(orderID),
			field.NewExecID(uuid.NewV4().String()),
			field.NewExecTransType(enum.ExecTransType_STATUS),
			field.NewExecType(execType),
			field.NewOrdStatus(ordStatus),
			field.NewSymbol(sym),
			field.NewSide(side),
			field.NewLeavesQty(remaining, 4), // qty
			field.NewCumQty(cumAmt, 4),
			AvgPxToFIX(avgPx),
		)
	case quickfix.BeginStringFIX44:
		e = fix44er.New(
			field.NewOrderID(orderID),
			field.NewExecID(uuid.NewV4().String()),
			field.NewExecType(execType),
			field.NewOrdStatus(ordStatus),
			field.NewSide(side),
			field.NewLeavesQty(remaining, 4), // qty
			field.NewCumQty(cumAmt, 4),
			AvgPxToFIX(avgPx),
		)
		e.Set(field.NewSymbol(sym))
	case quickfix.BeginStringFIXT11:
		e = fix50er.New(
			field.NewOrderID(orderID),
			field.NewExecID(uuid.NewV4().String()),
			field.NewExecType(execType),
			field.NewOrdStatus(ordStatus),
			field.NewSide(side),
			field.NewLeavesQty(remaining, 4), // qty
			field.NewCumQty(cumAmt, 4),
		)
		e.Set(field.NewSymbol(sym))
		e.Set(AvgPxToFIX(avgPx))
	default:
		panic(UnsupportedBeginStringText)
	}
	e.Set(field.NewAccount(account))
	if lastShares.Cmp(decimal.Zero) != 0 {
		e.Set(field.NewLastShares(lastShares, 4))
	}
	e.Set(field.NewOrderQty(amt, 4))
	if len(text) > 0 {
		e.Set(field.NewText(text))
	}
	if isMargin {
		e.Set(field.NewCashMargin(enum.CashMargin_MARGIN_CLOSE))
	}
	e.Set(field.NewOrdType(ordType))
	e.Set(field.NewClOrdID(clOrdID))

	if px != 0 && (ordType == enum.OrdType_LIMIT || ordType == enum.OrdType_STOP_LIMIT) {
		e.Set(field.NewPrice(decimal.NewFromFloat(px), 4))
	}
	if stop != 0 && (ordType == enum.OrdType_STOP || ordType == enum.OrdType_STOP_LIMIT) {
		e.Set(field.NewStopPx(decimal.NewFromFloat(stop), 4))
	}

	execInst := ""
	if trail != 0 {
		execInst = string(enum.ExecInst_PRIMARY_PEG)
		e.Set(field.NewPegDifference(decimal.NewFromFloat(trail), 4))
	}
	if flags&FlagHidden != 0 {
		e.Set(field.NewDisplayMethod(enum.DisplayMethod_UNDISCLOSED))
	}
	if flags&FlagPostOnly != 0 {
		execInst = execInst + string(enum.ExecInst_PARTICIPANT_DONT_INITIATE)
	}
	if len(execInst) > 0 {
		e.Set(field.NewExecInst(enum.ExecInst(execInst)))
	}
	e.Set(field.NewTimeInForce(tif))
	if tif == enum.TimeInForce_GOOD_TILL_DATE {
		e.Set(field.NewExpireTime(exp))
	}

	return e
}

// FIXExecutionReportFromOrder generates a FIX execution report from a bitfinex order
func FIXExecutionReportFromOrder(beginString string, o *bitfinex.Order, account string, execType enum.ExecType, cumQty float64, ordStatus enum.OrdStatus, text string, symbology symbol.Symbology, counterparty string, flags int, stop, peg float64) (e GenericFix) {
	orderID := strconv.FormatInt(o.ID, 10)
	// total order qty
	fAmt := o.Amount
	if fAmt < 0 {
		fAmt = -fAmt
	}
	ordtype, isMargin := OrdTypeToFIX(bitfinex.OrderType(o.Type))
	tif, exp := TimeInForceToFIX(bitfinex.OrderType(o.Type), o.MTSTif) // support FOK

	e = FIXExecutionReport(beginString, o.Symbol, strconv.FormatInt(o.CID, 10), orderID, account, execType, SideToFIX(o.Amount), fAmt, 0.0, cumQty, o.Price, stop, peg, o.PriceAvg, ordStatus, ordtype, isMargin, tif, exp, text, symbology, counterparty, flags)
	if len(text) > 0 {
		e.Set(field.NewText(text))
	}
	e.Set(field.NewLastShares(decimal.Zero, 4)) // qty
	return
}

// FIXExecutionReportFromTradeExecutionUpdate generates a FIX execution report from a bitfinex trade execution
func FIXExecutionReportFromTradeExecutionUpdate(beginString string, t *bitfinex.TradeExecutionUpdate, account, clOrdID string, origQty, totalFillQty, origPx, stopPx, trailPx, avgFillPx float64, symbology symbol.Symbology, counterparty string, expTif int64, flags int) (er GenericFix) {
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
	tif, exp := TimeInForceToFIX(bitfinex.OrderType(t.OrderType), expTif) // support FOK
	ordType, isMargin := OrdTypeToFIX(bitfinex.OrderType(t.OrderType))
	er = FIXExecutionReport(beginString, t.Pair, clOrdID, orderID, account, execType, SideToFIX(t.ExecAmount), origQty, execAmt, totalFillQty, origPx, stopPx, trailPx, avgFillPx, ordStatus, ordType, isMargin, tif, exp, "", symbology, counterparty, flags)
	f := t.Fee
	if f < 0 {
		f = -f
	}

	// trade-specific
	fee := decimal.NewFromFloat(f)
	er.Set(field.NewCommission(fee, 4))
	er.Set(field.NewCommType(enum.CommType_ABSOLUTE))
	er.Set(field.NewLastPx(decimal.NewFromFloat(t.ExecPrice), 4))
	return
}

func rejectReasonFromText(text string) enum.CxlRejReason {
	switch text {
	case OrderNotFoundText:
		return enum.CxlRejReason_UNKNOWN_ORDER
	}
	return enum.CxlRejReason_OTHER
}

// FIXOrderCancelReject generates a cancel reject message
func FIXOrderCancelReject(beginString, account, orderID, origClOrdID, cxlClOrdID, text string, isCancelReplace bool) (r GenericFix) {
	rejReason := rejectReasonFromText(text)
	if rejReason == enum.CxlRejReason_UNKNOWN_ORDER {
		orderID = "NONE" // FIX spec tag 37 in 35=9: If CxlRejReason="Unknown order", specify "NONE".
	}
	switch beginString {
	case quickfix.BeginStringFIX42:
		r = ocj42.New(
			field.NewOrderID(orderID),
			field.NewClOrdID(cxlClOrdID),
			field.NewOrigClOrdID(origClOrdID),
			field.NewOrdStatus(enum.OrdStatus_REJECTED),
			field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
		)
	case quickfix.BeginStringFIX44:
		r = ocj44.New(
			field.NewOrderID(orderID),
			field.NewClOrdID(cxlClOrdID),
			field.NewOrigClOrdID(origClOrdID),
			field.NewOrdStatus(enum.OrdStatus_REJECTED),
			field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
		)
	case quickfix.BeginStringFIXT11:
		r = ocj50.New(
			field.NewOrderID(orderID),
			field.NewClOrdID(cxlClOrdID),
			field.NewOrigClOrdID(origClOrdID),
			field.NewOrdStatus(enum.OrdStatus_REJECTED),
			field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
		)
	default:
		panic(UnsupportedBeginStringText)
	}
	r.Set(field.NewCxlRejReason(rejReason))
	r.Set(field.NewAccount(account))
	r.Set(field.NewText(text))
	responseTo := enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST
	if isCancelReplace {
		responseTo = enum.CxlRejResponseTo_ORDER_CANCEL_REPLACE_REQUEST
	}
	r.Set(field.NewCxlRejResponseTo(responseTo))
	return
}

// FIX42NoMDEntriesRepeatingGroupFromTradeTicker generates market data entries from ticker data
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
