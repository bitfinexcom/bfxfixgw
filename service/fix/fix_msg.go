package fix

import (
	"context"
	"errors"
	"fmt"
	"github.com/bitfinexcom/bfxfixgw/convert"
	"github.com/bitfinexcom/bfxfixgw/service/peer"
	"github.com/quickfixgo/tag"
	"log"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	lgout42 "github.com/quickfixgo/fix42/logout"
	mdr "github.com/quickfixgo/fix42/marketdatarequest"
	mdrr42 "github.com/quickfixgo/fix42/marketdatarequestreject"
	lgout44 "github.com/quickfixgo/fix44/logout"
	mdrr44 "github.com/quickfixgo/fix44/marketdatarequestreject"
	mdrr50 "github.com/quickfixgo/fix50/marketdatarequestreject"
	lgoutfixt "github.com/quickfixgo/fixt11/logout"
	"github.com/quickfixgo/quickfix"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
)

const (
	// PricePrecision is the FIX tag to specify a book subscription price precision
	PricePrecision quickfix.Tag = 20003

	// TagLeverage is the tag used for the leverage integer field
	TagLeverage quickfix.Tag = 20005
)

// Handle FIX42 messages and process them upstream to Bitfinex.

var rejectReasonOther = 0

func genFlags(hidden bool, postOnly bool) (flags int) {
	flags = 0
	if postOnly {
		flags = flags | bitfinex.OrderFlagPostOnly
	}
	if hidden {
		flags = flags | bitfinex.OrderFlagHidden
	}
	return
}

func genMTSTif(timeInForce string) int64 {
	if len(timeInForce) > 0 {
		tif, err := time.Parse(convert.TimeInForceFormat, timeInForce)
		if err != nil {
			panic(err)
		}
		return tif.UnixNano() / 1000000
	}
	return 0
}

func requestToOrder(o *bitfinex.OrderNewRequest) (ord *bitfinex.Order) {
	ord = &bitfinex.Order{
		GID:           o.GID,
		CID:           o.CID,
		Type:          o.Type,
		Symbol:        o.Symbol,
		Amount:        o.Amount,
		Price:         o.Price,
		PriceTrailing: o.PriceTrailing,
		PriceAuxLimit: o.PriceAuxLimit,
		Hidden:        o.Hidden,
		Flags:         int64(genFlags(o.Hidden, o.PostOnly)),
		MTSTif:        genMTSTif(o.TimeInForce),
	}
	return
}

func updateToOrder(o *bitfinex.OrderUpdateRequest, cid int64, typ, symbol string) (ord *bitfinex.Order) {
	ord = &bitfinex.Order{
		GID:           o.GID,
		CID:           cid,
		Type:          typ,
		Symbol:        symbol,
		Amount:        o.Amount,
		Price:         o.Price,
		PriceTrailing: o.PriceTrailing,
		PriceAuxLimit: o.PriceAuxLimit,
		Hidden:        o.Hidden,
		Flags:         int64(genFlags(o.Hidden, o.PostOnly)),
		MTSTif:        genMTSTif(o.TimeInForce),
	}
	return
}

func logout(message string, sID quickfix.SessionID) error {
	var msg convert.GenericFix
	switch sID.BeginString {
	case quickfix.BeginStringFIX42:
		msg = lgout42.New()
	case quickfix.BeginStringFIX44:
		msg = lgout44.New()
	case quickfix.BeginStringFIXT11:
		msg = lgoutfixt.New()
	default:
		return errors.New(convert.UnsupportedBeginStringText)
	}
	msg.Set(field.NewText(message))
	return quickfix.SendToTarget(msg, sID)
}

func sendToTarget(m quickfix.Messagable, sessionID quickfix.SessionID) quickfix.MessageRejectError {
	if err := quickfix.SendToTarget(m, sessionID); err != nil {
		return reject(err)
	}
	return nil
}

// OnFIXNewOrderSingle handles a New Order Single FIX message
func (f *FIX) OnFIXNewOrderSingle(msg quickfix.FieldMap, sID quickfix.SessionID) quickfix.MessageRejectError {
	p, ok := f.FindPeer(sID.String())
	if !ok {
		f.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("could not find established peer for session ID", rejectReasonOther, nil)
	}

	bo, err := convert.OrderNewFromFIXNewOrderSingle(msg, f.Symbology, sID.TargetCompID)
	if err != nil {
		return err
	}

	if lev := 0; msg.Has(TagLeverage) {
		if lev, err = msg.GetInt(TagLeverage); err != nil {
			return err
		}
		bo.Leverage = int64(lev)
	}

	ordtype := field.OrdTypeField{}
	if err = msg.Get(&ordtype); err != nil {
		return err
	}
	clordid := field.ClOrdIDField{}
	if err = msg.Get(&clordid); err != nil {
		return err
	}
	side := field.SideField{}
	if err = msg.Get(&side); err != nil {
		return err
	}
	var tif enum.TimeInForce
	tif, bo.TimeInForce, err = convert.GetTimeInForceFromFIX(msg)
	if err != nil {
		return err
	}
	ismargin := strings.Contains(bo.Type, "MARGIN")

	o := requestToOrder(bo)
	p.AddOrder(clordid.String(), bo.Price, bo.PriceAuxLimit, bo.PriceTrailing, bo.Amount, bo.Symbol, p.BfxUserID(), side.Value(), ordtype.Value(), ismargin, tif, o.MTSTif, int(o.Flags))
	// order has been accepted by business logic in gateway, no more 35=j

	e := p.Ws.SubmitOrder(context.Background(), bo)
	if e != nil {
		// should be an ER
		er := convert.FIXExecutionReportFromOrder(sID.BeginString, o, p.BfxUserID(), enum.ExecType_REJECTED, 0.0, enum.OrdStatus_REJECTED, e.Error(), f.Symbology, sID.TargetCompID, int(o.Flags), bo.PriceAuxLimit, bo.PriceTrailing)
		f.logger.Warn("could not submit order", zap.Error(e))
		return sendToTarget(er, sID)
	}

	return nil
}

// OnFIXOrderCancelReplaceRequest handles an Order Cancel Replace FIX message
func (f *FIX) OnFIXOrderCancelReplaceRequest(msg quickfix.FieldMap, sID quickfix.SessionID) quickfix.MessageRejectError {
	ocid := field.OrigClOrdIDField{} // required
	if err := msg.Get(&ocid); err != nil {
		return err
	}

	cid := field.ClOrdIDField{} // required
	if err := msg.Get(&cid); err != nil {
		return err
	}

	p, ok := f.FindPeer(sID.String())
	if !ok {
		f.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("could not find established peer for session ID", rejectReasonOther, nil)
	}

	id := ""
	var cache *peer.CachedOrder
	if msg.Has(tag.OrderID) {
		idField := field.OrderIDField{}
		if err := msg.Get(&idField); err != nil {
			return err
		}
		id = idField.String()
	} else {
		var er error
		if cache, er = p.LookupByClOrdID(ocid.String()); er != nil {
			r := convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), id, ocid.String(), cid.String(), convert.OrderNotFoundText, true)
			return sendToTarget(r, sID)
		}
		id = cache.OrderID
	}

	ou := &bitfinex.OrderUpdateRequest{GID: 0}
	//Ensure ids are fine
	cidi, er := strconv.ParseInt(cid.String(), 10, 64)
	if er != nil {
		r := convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), id, ocid.String(), cid.String(), convert.OrderNotFoundText, true)
		return sendToTarget(r, sID)
	} else if ou.ID, er = strconv.ParseInt(id, 10, 64); er != nil {
		r := convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), id, ocid.String(), cid.String(), convert.OrderNotFoundText, true)
		return sendToTarget(r, sID)
	} else if _, er = strconv.ParseInt(ocid.String(), 10, 64); er != nil {
		r := convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), id, ocid.String(), cid.String(), convert.OrderNotFoundText, true)
		return sendToTarget(r, sID)
	} else if cache == nil {
		cache, er = p.LookupByClOrdID(ocid.String())
		if er != nil || cache.OrderID != id {
			r := convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), id, ocid.String(), cid.String(), convert.OrderNotFoundText, true)
			return sendToTarget(r, sID)
		}
	}

	//Update requisite fields
	qty := field.OrderQtyField{}
	if err := msg.Get(&qty); err != nil {
		return err
	}
	ou.Amount = convert.GetAmountFromQtyAndSide(cache.Side, qty.Value())

	var t enum.OrdType
	var err quickfix.MessageRejectError
	if t, ou.Price, ou.PriceAuxLimit, ou.PriceTrailing, _, err = convert.GetPricesFromOrdType(msg); err != nil {
		return err
	}

	var tif enum.TimeInForce
	if tif, ou.TimeInForce, err = convert.GetTimeInForceFromFIX(msg); err != nil {
		return err
	}

	if lev := 0; msg.Has(TagLeverage) {
		if lev, err = msg.GetInt(TagLeverage); err != nil {
			return err
		}
		ou.Leverage = int64(lev)
	}

	ou.Hidden, ou.PostOnly, _ = convert.GetFlagsFromFIX(msg)

	typ, err := convert.OrderNewTypeFromFIX(msg)
	if err != nil {
		return err
	}
	o := updateToOrder(ou, cidi, typ, cache.Symbol)
	p.AddOrder(cid.String(), ou.Price, ou.PriceAuxLimit, ou.PriceTrailing, ou.Amount, cache.Symbol, p.BfxUserID(), cache.Side, t, cache.IsMargin, tif, genMTSTif(ou.TimeInForce), genFlags(ou.Hidden, ou.PostOnly))
	if _, er = p.UpdateOrder(cid.String(), id); er != nil {
		//Ensure order id is updated - this should not fail b/c above call inserts into cache
		panic(er)
	}

	// order has been accepted by business logic in gateway, no more 35=j
	e := p.Ws.SubmitUpdateOrder(context.Background(), ou)
	if e != nil {
		// should be an ER
		er := convert.FIXExecutionReportFromOrder(sID.BeginString, o, p.BfxUserID(), enum.ExecType_REJECTED, 0.0, enum.OrdStatus_REJECTED, e.Error(), f.Symbology, sID.TargetCompID, int(o.Flags), ou.PriceAuxLimit, ou.PriceTrailing)
		f.logger.Warn("could not submit order", zap.Error(e))
		return sendToTarget(er, sID)
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

func buildMarketDataRequestReject(beginString, mdReqID, text string, rejReason enum.MDReqRejReason) (rej convert.GenericFix) {
	switch beginString {
	case quickfix.BeginStringFIX42:
		rej = mdrr42.New(field.NewMDReqID(mdReqID))
	case quickfix.BeginStringFIX44:
		rej = mdrr44.New(field.NewMDReqID(mdReqID))
	case quickfix.BeginStringFIXT11:
		rej = mdrr50.New(field.NewMDReqID(mdReqID))
	default:
		panic(convert.UnsupportedBeginStringText)
	}
	rej.Set(field.NewText(text))
	rej.Set(field.NewMDReqRejReason(rejReason))
	return
}

// OnFIXMarketDataRequest handles a Market Data Request FIX message
func (f *FIX) OnFIXMarketDataRequest(msg quickfix.FieldMap, sID quickfix.SessionID) quickfix.MessageRejectError {
	p, ok := f.FindPeer(sID.String())
	if !ok {
		f.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("could not find established peer for session ID", rejectReasonOther, nil)
	}

	relSym := mdr.NewNoRelatedSymRepeatingGroup()
	if err := msg.GetGroup(relSym); err != nil {
		return err
	}

	if relSym.Len() <= 0 {
		f.logger.Warn("no symbol provided", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("no symbol provided", rejectReasonOther, nil)
	}

	mdReqID := field.MDReqIDField{}
	if err := msg.Get(&mdReqID); err != nil {
		return err
	}

	subType := field.SubscriptionRequestTypeField{}
	if err := msg.Get(&subType); err != nil {
		return err
	}

	depthField := field.MarketDepthField{}
	if err := msg.Get(&depthField); err != nil {
		return err
	}
	depth := depthField.Value()

	// validate depth
	if depth < 0 {
		return rejectError(fmt.Sprintf("invalid market depth for market data request: %d", depth))
	} else if 0 == depth {
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
			log.Printf("translate FIX %s to %s", fixSymbol, translated)
			symbol = translated
		} else {
			log.Printf("could not translate FIX %s: %s", fixSymbol, err2.Error())
			symbol = fixSymbol
		}
		// business logic has accepted message. after this return type-specific reject (MarketDataRequestReject)

		if p.MDReqIDExists(mdReqID.String()) {
			text := "duplicate MDReqID by session: " + mdReqID.String()
			rej := buildMarketDataRequestReject(sID.BeginString, mdReqID.String(), text, enum.MDReqRejReason_DUPLICATE_MDREQID)
			f.logger.Warn(text)
			return sendToTarget(rej, sID)
		}
		if _, has := p.LookupMDReqID(symbol); has {
			text := "duplicate symbol subscription for \"" + symbol + "\", one subscription per symbol allowed"
			rej := buildMarketDataRequestReject(sID.BeginString, mdReqID.String(), text, enum.MDReqRejReason_DUPLICATE_MDREQID)
			f.logger.Warn("duplicate symbol subscription by session: " + mdReqID.String())
			return sendToTarget(rej, sID)
		}

		// XXX: The following could most likely be abtracted to work both for 4.2 and 4.4.
		switch subType.Value() {
		default:
			text := fmt.Sprintf("subscription type not supported: %s", subType)
			rej := buildMarketDataRequestReject(sID.BeginString, mdReqID.String(), text, enum.MDReqRejReason_UNSUPPORTED_SUBSCRIPTIONREQUESTTYPE)
			f.logger.Warn(text)
			if errSend := sendToTarget(rej, sID); errSend != nil {
				return errSend
			}

		case enum.SubscriptionRequestType_SNAPSHOT:
			p.MapSymbolToReqID(symbol, mdReqID.String())
			bookSnapshot, err := p.Rest.Book.All(symbol, precision, depth)
			if err != nil {
				rej := buildMarketDataRequestReject(sID.BeginString, mdReqID.String(), err.Error(), enum.MDReqRejReason_UNKNOWN_SYMBOL)
				f.logger.Warn("could not get book snapshot: " + err.Error())
				return sendToTarget(rej, sID)
			}
			fix := convert.FIXMarketDataFullRefreshFromBookSnapshot(sID.BeginString, mdReqID.String(), bookSnapshot, f.Symbology, sID.TargetCompID)
			if errSend := sendToTarget(fix, sID); errSend != nil {
				return errSend
			}

		case enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES:
			p.MapSymbolToReqID(symbol, mdReqID.String())

			prec := bitfinex.Precision0
			if overridePrecision {
				prec = precision
			} else {
				aggregate := field.AggregatedBookField{} // aggregate by price (most granular by default) if no precision override is given
				if err = msg.Get(&aggregate); err == nil && !aggregate.Value() {
					prec = bitfinex.PrecisionRawBook
				}
			}
			bookReqID, err := p.Ws.SubscribeBook(context.Background(), symbol, prec, bitfinex.FrequencyRealtime, depth)
			if err != nil {
				rej := buildMarketDataRequestReject(sID.BeginString, mdReqID.String(), err.Error(), enum.MDReqRejReason_UNKNOWN_SYMBOL)
				f.logger.Warn("could not subscribe to book: " + err.Error())
				return sendToTarget(rej, sID)
			}
			tradeReqID, err := p.Ws.SubscribeTrades(context.Background(), symbol)
			if err != nil {
				if errUnsub := p.Ws.Unsubscribe(context.Background(), bookReqID); errUnsub != nil { // remove book subscription
					err = errors.New(err.Error() + " occurred, and also unable to subscribe due to " + errUnsub.Error())
				}
				rej := buildMarketDataRequestReject(sID.BeginString, mdReqID.String(), err.Error(), enum.MDReqRejReason_UNKNOWN_SYMBOL)
				f.logger.Warn("could not subscribe to trades: " + err.Error())
				return sendToTarget(rej, sID)
			}
			f.logger.Info("mapping FIX->API request ID", zap.String("MDReqID", mdReqID.String()), zap.String("BookReqID", bookReqID), zap.String("TradeReqID", tradeReqID))
			p.MapMDReqIDs(mdReqID.String(), bookReqID, tradeReqID)

		case enum.SubscriptionRequestType_DISABLE_PREVIOUS_SNAPSHOT_PLUS_UPDATE_REQUEST:
			if bookReqID, tradeReqID, ok := p.LookupAPIReqIDs(mdReqID.String()); ok {
				f.logger.Info("unsubscribe from API", zap.String("MDReqID", mdReqID.String()), zap.String("BookReqID", bookReqID), zap.String("TradeReqID", tradeReqID))
				errUnsubBook := p.Ws.Unsubscribe(context.Background(), bookReqID)
				errUnsubTrade := p.Ws.Unsubscribe(context.Background(), tradeReqID)
				if errUnsubBook != nil || errUnsubTrade != nil {
					errMsg := fmt.Sprintf("Unsubscribe book / trade errors: %v / %v", errUnsubBook, errUnsubTrade)
					return reject(errors.New(errMsg))
				}
				return nil
			}
			text := "could not find subscription for MDReqID: " + mdReqID.String()
			rej := buildMarketDataRequestReject(sID.BeginString, mdReqID.String(), text, enum.MDReqRejReason_UNKNOWN_SYMBOL)
			f.logger.Warn(text)
			if err := sendToTarget(rej, sID); err != nil {
				return err
			}
		}
	}

	return nil
}

// OnFIXOrderCancelRequest handles an Order Cancel message
func (f *FIX) OnFIXOrderCancelRequest(msg quickfix.FieldMap, sID quickfix.SessionID) quickfix.MessageRejectError {
	ocid := field.OrigClOrdIDField{} // required
	if err := msg.Get(&ocid); err != nil {
		return err
	}

	cid := field.ClOrdIDField{} // required
	if err := msg.Get(&cid); err != nil {
		return err
	}

	// The spec says that a quantity and side are also required but the bitfinex API does not
	// care about either of those for cancelling.
	txnT := field.TransactTimeField{}
	if err := msg.Get(&txnT); err != nil {
		return err
	}

	oc := &bitfinex.OrderCancelRequest{}

	p, ok := f.FindPeer(sID.String())
	if !ok {
		f.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return quickfix.NewMessageRejectError("could not find established peer for session ID", rejectReasonOther, nil)
	}

	id := ""
	if msg.Has(tag.OrderID) { // cancel by server-assigned ID
		idField := field.OrderIDField{}
		if err := msg.Get(&idField); err != nil {
			return err
		}
		id = idField.Value()
		idi, err := strconv.ParseInt(id, 10, 64)
		if err != nil { // bitfinex uses int IDs so we can reject right away.
			r := convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), id, ocid.Value(), cid.Value(), convert.OrderNotFoundText, false)
			return sendToTarget(r, sID)
		}
		oc.ID = idi
	} else { // cancel by client-assigned ID
		ocidi, err := strconv.ParseInt(ocid.Value(), 10, 64)
		if err != nil {
			r := convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), id, ocid.Value(), cid.Value(), convert.OrderNotFoundText, false)
			return sendToTarget(r, sID)
		}
		oc.CID = ocidi
		d := txnT.Format("2006-01-02")
		oc.CIDDate = d
		cache, err := p.LookupByClOrdID(ocid.Value())
		if err == nil {
			id = cache.OrderID
		}
	}

	if err2 := p.Ws.Send(context.Background(), oc); err2 != nil {
		f.logger.Error("not logged onto websocket", zap.String("SessionID", sID.String()), zap.Error(err2))
		rej := convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), id, ocid.Value(), cid.Value(), err2.Error(), false)
		return sendToTarget(rej, sID)
	}

	return nil
}

// OnFIXOrderStatusRequest handles a FIX order status request
func (f *FIX) OnFIXOrderStatusRequest(msg quickfix.FieldMap, sID quickfix.SessionID) quickfix.MessageRejectError {
	oid := field.OrderIDField{}
	if err := msg.Get(&oid); err != nil {
		return err
	}
	/*
		cid, err := msg.GetClOrdID()
		if err != nil {
			return err
		}
	*/
	oidi, nerr := strconv.ParseInt(oid.Value(), 10, 64)
	if nerr != nil {
		return reject(nerr)
	}

	foundPeer, ok := f.FindPeer(sID.String())
	if !ok {
		return reject(fmt.Errorf("could not find route for FIX session %s", sID.String()))
	}

	order, nerr := foundPeer.Rest.Orders.GetByOrderId(oidi)
	if nerr != nil {
		return reject(nerr)
	}
	orderID := strconv.FormatInt(order.ID, 10)
	clOrdID := strconv.FormatInt(order.CID, 10)
	ordtype := bitfinex.OrderType(order.Type)
	tif, _ := convert.TimeInForceToFIX(ordtype, order.MTSTif)
	cached, err2 := foundPeer.LookupByOrderID(orderID)
	if err2 != nil {
		ot, isMargin := convert.OrdTypeToFIX(ordtype)
		cached = foundPeer.AddOrder(clOrdID, order.Price, order.PriceAuxLimit, order.PriceTrailing, order.Amount, order.Symbol, foundPeer.BfxUserID(), convert.SideToFIX(order.Amount), ot, isMargin, tif, order.MTSTif, int(order.Flags))
	}
	status := convert.OrdStatusToFIX(order.Status)
	er := convert.FIXExecutionReportFromOrder(sID.BeginString, order, foundPeer.BfxUserID(), enum.ExecType_ORDER_STATUS, cached.FilledQty(), status, "", f.Symbology, sID.TargetCompID, cached.Flags, cached.Stop, cached.Trail)
	return sendToTarget(er, sID)
}
