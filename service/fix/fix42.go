package fix

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bitfinexcom/bfxfixgw/convert"

	"go.uber.org/zap"
	lg "log"

	//er "github.com/quickfixgo/quickfix/fix42/executionreport"
	mdr "github.com/quickfixgo/fix42/marketdatarequest"
	mdrr "github.com/quickfixgo/fix42/marketdatarequestreject"
	nos "github.com/quickfixgo/fix42/newordersingle"
	ocj "github.com/quickfixgo/fix42/ordercancelreject"
	ocr "github.com/quickfixgo/fix42/ordercancelrequest"
	osr "github.com/quickfixgo/fix42/orderstatusrequest"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/quickfixgo/quickfix"

	bitfinex "github.com/bitfinexcom/bitfinex-api-go/v2"
)

const (
	PricePrecision quickfix.Tag = 20003
)

// Handle FIX42 messages and process them upstream to Bitfinex.

var rejectReasonOther = 0

func (f *FIX) OnFIX42NewOrderSingle(msg nos.NewOrderSingle, sID quickfix.SessionID) quickfix.MessageRejectError {
	p, ok := f.FindPeer(sID.String())
	if !ok {
		f.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("could not find established peer for session ID", rejectReasonOther, nil)
	}

	bo, err := convert.OrderNewFromFIX42NewOrderSingle(msg)
	if err != nil {
		return err
	}

	lg.Printf("submit order %p", p.Ws)

	ordtype, _ := msg.GetOrdType()
	clordid, _ := msg.GetClOrdID()
	side, _ := msg.GetSide()
	p.AddOrder(clordid, bo.Price, bo.Amount, bo.Symbol, p.BfxUserID(), side, ordtype)

	e := p.Ws.SubmitOrder(context.Background(), bo)
	if e != nil {
		f.logger.Warn("could not submit order", zap.Error(e))
	} else {

	}

	return nil
}

func reject(err error) quickfix.MessageRejectError {
	return quickfix.NewMessageRejectError(err.Error(), rejectReasonOther, nil)
}

func rejectError(msg string) quickfix.MessageRejectError {
	return quickfix.NewBusinessMessageRejectError(msg, rejectReasonOther, nil)
}

func validatePrecision(prec string) (bitfinex.BookPrecision, bool) {
	switch prec {
	case string(bitfinex.Precision0):
		return bitfinex.Precision0, true
	case string(bitfinex.Precision1):
		return bitfinex.Precision1, true
	case string(bitfinex.Precision2):
		return bitfinex.Precision2, true
	case string(bitfinex.Precision3):
		return bitfinex.Precision3, true
	case string(bitfinex.PrecisionRawBook):
		return bitfinex.PrecisionRawBook, true
	}
	return bitfinex.Precision0, false
}

func (f *FIX) OnFIX42MarketDataRequest(msg mdr.MarketDataRequest, sID quickfix.SessionID) quickfix.MessageRejectError {

	p, ok := f.FindPeer(sID.String())
	if !ok {
		f.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("could not find established peer for session ID", rejectReasonOther, nil)
	}

	relSym, err := msg.GetNoRelatedSym()
	if err != nil {
		return err
	}

	if relSym.Len() <= 0 {
		f.logger.Warn("no symbol provided", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("no symbol provided", rejectReasonOther, nil)
	}

	mdReqID, err := msg.GetMDReqID()
	if err != nil {
		return err
	}

	subType, err := msg.GetSubscriptionRequestType()
	if err != nil {
		return err
	}

	depth, err := msg.GetMarketDepth()
	if err != nil {
		return err
	}
	// validate depth
	if depth < 0 {
		return rejectError(fmt.Sprintf("invalid market depth for market data request: %d", depth))
	}

	var precision bitfinex.BookPrecision
	var overridePrecision bool
	fixPrecision, err := msg.GetString(PricePrecision)
	if err != nil {
		precision = bitfinex.Precision0
	} else {
		var ok bool
		precision, overridePrecision = validatePrecision(fixPrecision)
		if !ok {
			return rejectError(fmt.Sprintf("invalid precision for market data request: %s", fixPrecision))
		}
	}

	for i := 0; i < relSym.Len(); i++ {

		symbol, err := relSym.Get(i).GetSymbol()
		if err != nil {
			return err
		}

		// XXX: The following could most likely be abtracted to work both for 4.2 and 4.4.
		switch subType {
		default:
			rej := mdrr.New(field.NewMDReqID(mdReqID))
			text := fmt.Sprintf("subscription type not supported: %s", subType)
			f.logger.Warn(text)
			rej.SetText(text)
			quickfix.SendToTarget(rej, sID)

		case enum.SubscriptionRequestType_SNAPSHOT:
			p.MapSymbolToReqID(symbol, mdReqID)
			bookSnapshot, err := p.Rest.Book.All(symbol, precision, depth)
			if err != nil {
				return reject(err)
			}
			fix := convert.FIX42MarketDataFullRefreshFromBookSnapshot(mdReqID, bookSnapshot, f.Symbology, sID.SenderCompID)
			quickfix.SendToTarget(fix, sID)
			tradeSnapshot, err := p.Rest.Trades.All(symbol)
			if err != nil {
				return reject(err)
			}
			fix = convert.FIX42MarketDataFullRefreshFromTradeSnapshot(mdReqID, tradeSnapshot, f.Symbology, sID.SenderCompID)
			quickfix.SendToTarget(fix, sID)

		case enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES:
			p.MapSymbolToReqID(symbol, mdReqID)

			prec := bitfinex.PrecisionRawBook
			if overridePrecision {
				prec = precision
			} else {
				aggregate, _ := msg.GetAggregatedBook() // aggregate by price (most granular by default) if no precision override is given
				if aggregate {
					prec = bitfinex.Precision0
				}
			}
			bookReqID, err := p.Ws.SubscribeBook(context.Background(), symbol, prec, bitfinex.FrequencyRealtime, depth)
			if err != nil {
				return reject(err)
			}
			tradeReqID, err := p.Ws.SubscribeTrades(context.Background(), symbol)
			if err != nil {
				p.Ws.Unsubscribe(context.Background(), bookReqID) // remove book subscription
				return reject(err)
			}
			f.logger.Info("mapping FIX->API request ID", zap.String("MDReqID", mdReqID), zap.String("BookReqID", bookReqID), zap.String("TradeReqID", tradeReqID))
			p.MapMDReqIDs(mdReqID, bookReqID, tradeReqID)

		case enum.SubscriptionRequestType_DISABLE_PREVIOUS_SNAPSHOT_PLUS_UPDATE_REQUEST:
			if bookReqID, tradeReqID, ok := p.LookupAPIReqIDs(mdReqID); ok {
				f.logger.Info("unsubscribe from API", zap.String("MDReqID", mdReqID), zap.String("BookReqID", bookReqID), zap.String("TradeReqID", tradeReqID))
				p.Ws.Unsubscribe(context.Background(), bookReqID)
				p.Ws.Unsubscribe(context.Background(), tradeReqID)
			}
			return rejectError(fmt.Sprintf("could not find subscription with ID \"%s\"", mdReqID))
		}
	}

	return nil
}

func (f *FIX) OnFIX42OrderCancelRequest(msg ocr.OrderCancelRequest, sID quickfix.SessionID) quickfix.MessageRejectError {
	ocid, err := msg.GetOrigClOrdID() // Required
	if err != nil {
		return err
	}

	cid, _ := msg.GetClOrdID()

	id, _ := msg.GetOrderID()

	// The spec says that a quantity and side are also required but the bitfinex API does not
	// care about either of those for cancelling.
	txnT, _ := msg.GetTransactTime()

	oc := &bitfinex.OrderCancelRequest{}

	p, ok := f.FindPeer(sID.String())
	if !ok {
		f.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("could not find established peer for session ID", rejectReasonOther, nil)
	}

	if id != "" {
		idi, err := strconv.ParseInt(id, 10, 64)
		if err != nil { // bitfinex uses int IDs so we can reject right away.
			r := ocj.New(
				field.NewOrderID(id),
				field.NewClOrdID(cid),
				field.NewOrigClOrdID(ocid),
				field.NewOrdStatus(enum.OrdStatus_REJECTED),
				field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
			)
			r.SetCxlRejReason(enum.CxlRejReason_UNKNOWN_ORDER)
			r.SetAccount(p.BfxUserID())
			quickfix.SendToTarget(r, sID)
			return nil
		}
		oc.ID = idi
	} else {
		ocidi, err := strconv.ParseInt(ocid, 10, 64)
		if err != nil {
			r := ocj.New(
				field.NewOrderID(id),
				field.NewClOrdID(cid),
				field.NewOrigClOrdID(ocid),
				field.NewOrdStatus(enum.OrdStatus_REJECTED),
				field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
			)
			r.SetCxlRejReason(enum.CxlRejReason_UNKNOWN_ORDER)
			r.SetAccount(p.BfxUserID())
			quickfix.SendToTarget(r, sID)
			return nil
		}
		oc.CID = ocidi
		d := txnT.Format("2006-01-02")
		oc.CIDDate = d
	}
	if p.Ws.IsConnected() {
		p.Ws.Send(context.Background(), oc)
	} else {
		// TODO still getting heartbeats, where is this connection??
		// ord vs. market data host?
		f.logger.Error("not logged onto websocket", zap.String("SessionID", sID.String()))
	}

	return nil
}

func (f *FIX) OnFIX42OrderStatusRequest(msg osr.OrderStatusRequest, sID quickfix.SessionID) quickfix.MessageRejectError {
	oid, err := msg.GetOrderID()
	if err != nil {
		return err
	}
	/*
		cid, err := msg.GetClOrdID()
		if err != nil {
			return err
		}
	*/
	oidi, nerr := strconv.ParseInt(oid, 10, 64)

	peer, ok := f.FindPeer(sID.String())
	if !ok {
		return reject(fmt.Errorf("could not find route for FIX session %s", sID.String()))
	}

	order, nerr := peer.Rest.Orders.Status(oidi)
	if nerr != nil {
		r := quickfix.NewBusinessMessageRejectError(nerr.Error(), 0 /*OTHER*/, nil)
		return r
	}
	orderID := strconv.FormatInt(order.ID, 10)
	clOrdID := strconv.FormatInt(order.CID, 10)
	cached, err2 := peer.LookupByOrderID(orderID)
	if err2 != nil {
		cached = peer.AddOrder(clOrdID, order.Price, order.Amount, order.Symbol, peer.BfxUserID(), convert.SideToFIX(order.Amount), convert.OrdTypeToFIX(order.Type))
	}
	status := convert.OrdStatusToFIX(order.Status)
	er := convert.FIX42ExecutionReportFromOrder(order, peer.BfxUserID(), enum.ExecType_ORDER_STATUS, cached.FilledQty(), status, "", f.Symbology, sID.SenderCompID)
	quickfix.SendToTarget(er, sID)

	return nil
}
