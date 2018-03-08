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

	bitfinex "github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/bitfinexcom/bitfinex-api-go/v2/rest"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
)

const (
	PricePrecision quickfix.Tag = 20100
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

	e := p.Ws.SubmitOrder(context.Background(), bo)
	if e != nil {
		f.logger.Warn("could not submit order", zap.Error(e))
	}

	return nil
}

func reject(err error) quickfix.MessageRejectError {
	return quickfix.NewMessageRejectError(err.Error(), rejectReasonOther, nil)
}

func makeReject(msg string) quickfix.MessageRejectError {
	return quickfix.NewBusinessMessageRejectError(msg, rejectReasonOther, nil)
}

func validatePrecision(prec string) (rest.BookPrecision, bool) {
	switch prec {
	case string(rest.Precision0):
		return rest.Precision0, true
	case string(rest.Precision1):
		return rest.Precision1, true
	case string(rest.Precision2):
		return rest.Precision2, true
	case string(rest.Precision3):
		return rest.Precision3, true
	case string(rest.PrecisionRawBook):
		return rest.PrecisionRawBook, true
	}
	return rest.Precision0, false
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

	var precision rest.BookPrecision
	fixPrecision, err := msg.GetString(PricePrecision)
	if err != nil {
		precision = rest.Precision0
	} else {
		var ok bool
		precision, ok = validatePrecision(fixPrecision)
		if !ok {
			return makeReject(fmt.Sprintf("invalid precision for market data request: %s", fixPrecision))
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
			snapshot, err := p.Rest.Book.All(symbol, precision, depth)
			if err != nil {
				return reject(err)
			}
			fix := convert.FIX42MarketDataFullRefreshFromBookSnapshot(mdReqID, snapshot)
			quickfix.SendToTarget(fix, sID)

		case enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES:
			// TODO manage subscription
			// TODO price levels
			p.Ws.SubscribeBook(context.Background(), symbol, websocket.PrecisionRawBook, websocket.FrequencyRealtime, 25)

		case enum.SubscriptionRequestType_DISABLE_PREVIOUS_SNAPSHOT_PLUS_UPDATE_REQUEST:
			// TODO unsubscribe
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
	//txnT, _ := msg.GetTransactTime()

	oc := &bitfinex.OrderCancelRequest{}

	peer, ok := f.FindPeer(sID.String())
	if !ok {
		return reject(fmt.Errorf("could not find route for FIX session %s", sID.String()))
	}

	if id != "" {
		//idi, err := strconv.ParseInt(id, 10, 64)
		if err != nil { // bitfinex uses int IDs so we can reject right away.
			r := ocj.New(
				field.NewOrderID(id),
				field.NewClOrdID(cid),
				field.NewOrigClOrdID(ocid),
				field.NewOrdStatus(enum.OrdStatus_REJECTED),
				field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
			)
			r.SetCxlRejReason(enum.CxlRejReason_UNKNOWN_ORDER)
			r.SetAccount(peer.BfxUserID())
			quickfix.SendToTarget(r, sID)
			return nil
		}
		//oc.ID = &idi
	} else {
		//ocidi, err := strconv.ParseInt(ocid, 10, 64)
		if err != nil {
			r := ocj.New(
				field.NewOrderID(id),
				field.NewClOrdID(cid),
				field.NewOrigClOrdID(ocid),
				field.NewOrdStatus(enum.OrdStatus_REJECTED),
				field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
			)
			r.SetCxlRejReason(enum.CxlRejReason_UNKNOWN_ORDER)
			r.SetAccount(peer.BfxUserID())
			quickfix.SendToTarget(r, sID)
			return nil
		}
		//oc.CID = &ocidi
		//d := txnT.Format("2006-01-02")
		//oc.CIDDate = &d
	}
	peer.Ws.Send(context.Background(), oc)

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

	er := convert.FIX42ExecutionReportFromOrder(order, peer.BfxUserID(), enum.ExecType_ORDER_STATUS)
	quickfix.SendToTarget(er, sID)

	return nil
}
