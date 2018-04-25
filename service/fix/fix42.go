package fix

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bitfinexcom/bfxfixgw/convert"

	"go.uber.org/zap"

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

	lg "log"

	bitfinex "github.com/bitfinexcom/bitfinex-api-go/v2"
)

const (
	// PricePrecision is the FIX tag to specify a book subscription price precision
	PricePrecision quickfix.Tag = 20003
)

// Handle FIX42 messages and process them upstream to Bitfinex.

var rejectReasonOther = 0

func requestToOrder(o *bitfinex.OrderNewRequest) *bitfinex.Order {
	flags := 0
	if o.PostOnly {
		flags = flags | bitfinex.OrderFlagPostOnly
	}
	if o.Hidden {
		flags = flags | bitfinex.OrderFlagHidden
	}
	return &bitfinex.Order{
		GID:           o.GID,
		CID:           o.CID,
		Type:          o.Type,
		Symbol:        o.Symbol,
		Amount:        o.Amount,
		Price:         o.Price,
		PriceTrailing: o.PriceTrailing,
		PriceAuxLimit: o.PriceAuxLimit,
		Hidden:        o.Hidden,
		Flags:         int64(flags),
	}
}

func requestToCxl(o *bitfinex.OrderCancelRequest) *bitfinex.OrderCancel {
	return &bitfinex.OrderCancel{
		ID:  o.ID,
		CID: o.CID,
	}
}

func (f *FIX) OnFIX42NewOrderSingle(msg nos.NewOrderSingle, sID quickfix.SessionID) quickfix.MessageRejectError {
	p, ok := f.FindPeer(sID.String())
	if !ok {
		f.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("could not find established peer for session ID", rejectReasonOther, nil)
	}

	bo, err := convert.OrderNewFromFIX42NewOrderSingle(msg, f.Symbology, sID.TargetCompID)
	if err != nil {
		return err
	}

	ordtype, _ := msg.GetOrdType()
	clordid, _ := msg.GetClOrdID()
	side, _ := msg.GetSide()
	tif, err := msg.GetTimeInForce()
	if err != nil {
		tif = enum.TimeInForce_GOOD_TILL_CANCEL // default TIF
	}

	flags := 0
	if bo.Hidden {
		flags = flags | convert.FlagHidden
	}
	if bo.PostOnly {
		flags = flags | convert.FlagPostOnly
	}
	p.AddOrder(clordid, bo.Price, bo.PriceAuxLimit, bo.PriceTrailing, bo.Amount, bo.Symbol, p.BfxUserID(), side, ordtype, tif, flags)
	// order has been accepted by business logic in gateway, no more 35=j

	e := p.Ws.SubmitOrder(context.Background(), bo)
	if e != nil {
		// should be an ER
		o := requestToOrder(bo)
		er := convert.FIX42ExecutionReportFromOrder(o, p.BfxUserID(), enum.ExecType_REJECTED, 0.0, enum.OrdStatus_REJECTED, e.Error(), f.Symbology, sID.TargetCompID, flags, bo.PriceAuxLimit, bo.PriceTrailing)
		f.logger.Warn("could not submit order", zap.Error(e))
		quickfix.SendToTarget(er, sID)
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
	if 0 == depth {
		depth = 100
	}

	var precision bitfinex.BookPrecision
	var overridePrecision bool
	fixPrecision, err := msg.GetString(PricePrecision)
	if err != nil {
		precision = bitfinex.Precision0
	} else {
		precision, overridePrecision = validatePrecision(fixPrecision)
		if !overridePrecision {
			return rejectError(fmt.Sprintf("invalid precision for market data request: %s", fixPrecision))
		}
	}

	for i := 0; i < relSym.Len(); i++ {

		fixSymbol, err := relSym.Get(i).GetSymbol()
		if err != nil {
			return err
		}
		translated, err2 := f.Symbology.ToBitfinex(fixSymbol, sID.TargetCompID)
		var symbol string
		if err2 == nil {
			lg.Printf("translate FIX %s to %s", fixSymbol, translated)
			symbol = translated
		} else {
			lg.Printf("could not translate FIX %s: %s", fixSymbol, err2.Error())
			symbol = fixSymbol
		}
		// business logic has accepted message. after this return type-specific reject (MarketDataRequestReject)

		if p.MDReqIDExists(mdReqID) {
			rej := mdrr.New(field.NewMDReqID(mdReqID))
			rej.SetText(err.Error())
			rej.SetMDReqRejReason(enum.MDReqRejReason_DUPLICATE_MDREQID)
			f.logger.Warn("duplicate MDReqID by session: " + mdReqID)
			quickfix.SendToTarget(rej, sID)
			return nil
		}
		if _, has := p.LookupMDReqID(symbol); has {
			rej := mdrr.New(field.NewMDReqID(mdReqID))
			rej.SetText("duplicate symbol subscription for \"" + symbol + "\", one subscription per symbol allowed")
			f.logger.Warn("duplicate symbol subscription by session: " + mdReqID)
			quickfix.SendToTarget(rej, sID)
			return nil
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
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				rej.SetText(err.Error())
				f.logger.Warn("could not get book snapshot: " + err.Error())
				quickfix.SendToTarget(rej, sID)
				return nil
			}
			fix := convert.FIX42MarketDataFullRefreshFromBookSnapshot(mdReqID, bookSnapshot, f.Symbology, sID.TargetCompID)
			quickfix.SendToTarget(fix, sID)

		case enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES:
			p.MapSymbolToReqID(symbol, mdReqID)

			prec := bitfinex.Precision0
			if overridePrecision {
				prec = precision
			} else {
				aggregate, err := msg.GetAggregatedBook() // aggregate by price (most granular by default) if no precision override is given
				if err == nil && !aggregate {
					prec = bitfinex.PrecisionRawBook
				}
			}
			bookReqID, err := p.Ws.SubscribeBook(context.Background(), symbol, prec, bitfinex.FrequencyRealtime, depth)
			if err != nil {
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				rej.SetText(err.Error())
				f.logger.Warn("could not subscribe to book: " + err.Error())
				quickfix.SendToTarget(rej, sID)
				return nil
			}
			tradeReqID, err := p.Ws.SubscribeTrades(context.Background(), symbol)
			if err != nil {
				p.Ws.Unsubscribe(context.Background(), bookReqID) // remove book subscription
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				rej.SetText(err.Error())
				f.logger.Warn("could not subscribe to trades: " + err.Error())
				quickfix.SendToTarget(rej, sID)
				return nil
			}
			f.logger.Info("mapping FIX->API request ID", zap.String("MDReqID", mdReqID), zap.String("BookReqID", bookReqID), zap.String("TradeReqID", tradeReqID))
			p.MapMDReqIDs(mdReqID, bookReqID, tradeReqID)

		case enum.SubscriptionRequestType_DISABLE_PREVIOUS_SNAPSHOT_PLUS_UPDATE_REQUEST:
			if bookReqID, tradeReqID, ok := p.LookupAPIReqIDs(mdReqID); ok {
				f.logger.Info("unsubscribe from API", zap.String("MDReqID", mdReqID), zap.String("BookReqID", bookReqID), zap.String("TradeReqID", tradeReqID))
				p.Ws.Unsubscribe(context.Background(), bookReqID)
				p.Ws.Unsubscribe(context.Background(), tradeReqID)
				return nil
			}
			rej := mdrr.New(field.NewMDReqID(mdReqID))
			rej.SetText("could not find subscription for MDReqID: " + mdReqID)
			f.logger.Warn("could not find subscription for MDReqID: " + mdReqID)
			quickfix.SendToTarget(rej, sID)
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

	err2 := p.Ws.Send(context.Background(), oc)
	if err2 != nil {
		f.logger.Error("not logged onto websocket", zap.String("SessionID", sID.String()), zap.Error(err))
		ocr := convert.FIX42OrderCancelReject(p.BfxUserID(), id, ocid, cid, err2.Error())
		quickfix.SendToTarget(ocr, sID)
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
		return reject(nerr)
	}
	orderID := strconv.FormatInt(order.ID, 10)
	clOrdID := strconv.FormatInt(order.CID, 10)
	ordtype := bitfinex.OrderType(order.Type)
	tif := convert.TimeInForceToFIX(ordtype)
	cached, err2 := peer.LookupByOrderID(orderID)
	if err2 != nil {
		cached = peer.AddOrder(clOrdID, order.Price, order.PriceAuxLimit, order.PriceTrailing, order.Amount, order.Symbol, peer.BfxUserID(), convert.SideToFIX(order.Amount), convert.OrdTypeToFIX(ordtype), tif, int(order.Flags))
	}
	status := convert.OrdStatusToFIX(order.Status)
	er := convert.FIX42ExecutionReportFromOrder(order, peer.BfxUserID(), enum.ExecType_ORDER_STATUS, cached.FilledQty(), status, "", f.Symbology, sID.TargetCompID, cached.Flags, cached.Stop, cached.Trail)
	quickfix.SendToTarget(er, sID)

	return nil
}
