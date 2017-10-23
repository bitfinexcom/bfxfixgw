package fix

import (
	"context"
	"strconv"
	"time"

	"github.com/bitfinexcom/bfxfixgw/convert"

	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"

	//er "github.com/quickfixgo/quickfix/fix44/executionreport"
	mdr "github.com/quickfixgo/fix44/marketdatarequest"
	mdrr "github.com/quickfixgo/fix44/marketdatarequestreject"
	mdsfr "github.com/quickfixgo/fix44/marketdatasnapshotfullrefresh"
	nos "github.com/quickfixgo/fix44/newordersingle"
	ocj "github.com/quickfixgo/fix44/ordercancelreject"
	ocr "github.com/quickfixgo/fix44/ordercancelrequest"
	osr "github.com/quickfixgo/fix44/orderstatusrequest"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
)

func (f *FIX) FIX44Handler(o interface{}, sID quickfix.SessionID) {
	f.logger.Debug("in FIX44TermDataHandler", zap.Any("object", o))

	switch d := o.(type) {
	case bitfinex.OrderSnapshot: // Order snapshot
		f.FIX44OrderSnapshotHandler(d, sID)
	case bitfinex.OrderNew: // Order new
		f.FIX44OrderNewHandler(d, sID)
	case bitfinex.OrderCancel: // Order cancel
		f.FIX44OrderCancelHandler(d, sID)
	case bitfinex.Notification: // Notification
		f.FIX44NotificationHandler(d, sID)
	default: // unknown
		return
	}
}

func (f *FIX) FIX44NotificationHandler(d bitfinex.Notification, sID quickfix.SessionID) {
	switch o := d.NotifyInfo.(type) {
	case bitfinex.OrderCancel:
		// Only handling error currently.
		if d.Status == "ERROR" && d.Text == "Order not found." {
			// Send out an OrderCancelReject
			r := ocj.New(
				field.NewOrderID("NONE"),
				field.NewClOrdID("NONE"), // XXX: This should be the actual ClOrdID which we don't have in this context.
				field.NewOrigClOrdID(strconv.FormatInt(o.CID, 10)),
				field.NewOrdStatus(enum.OrdStatus_REJECTED),
				field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
			)
			r.SetCxlRejReason(enum.CxlRejReason_UNKNOWN_ORDER)
			r.SetAccount(f.bfxUserID)
			quickfix.SendToTarget(r, sID)
			return
		}
		return
	case bitfinex.OrderNew:
		// XXX: Handle this at some point.
		return
	default:
		return
	}
}

func (f *FIX) FIX44OrderSnapshotHandler(os bitfinex.OrderSnapshot, sID quickfix.SessionID) {
	for _, o := range os {
		er := convert.FIX44ExecutionReportFromOrder(bitfinex.Order(o))
		er.SetAccount(f.bfxUserID)
		er.SetExecType(enum.ExecType_ORDER_STATUS)
		quickfix.SendToTarget(er, sID)
	}
	return
}

func (f *FIX) FIX44OrderNewHandler(o bitfinex.OrderNew, sID quickfix.SessionID) {
	er := convert.FIX44ExecutionReportFromOrder(bitfinex.Order(o))
	er.SetAccount(f.bfxUserID)
	quickfix.SendToTarget(er, sID)
	return
}

func (f *FIX) FIX44OrderCancelHandler(o bitfinex.OrderCancel, sID quickfix.SessionID) {
	er := convert.FIX44ExecutionReportFromOrder(bitfinex.Order(o))
	er.SetExecType(enum.ExecType_CANCELED)
	er.SetAccount(f.bfxUserID)
	quickfix.SendToTarget(er, sID)
	return
}

func (f *FIX) OnFIX44NewOrderSingle(msg nos.NewOrderSingle, sID quickfix.SessionID) quickfix.MessageRejectError {
	bo, err := convert.OrderNewFromFIX44NewOrderSingle(msg)
	if err != nil {
		return err
	}

	go func() {
		// XXX: handle error?
		f.bfx.Websocket.Send(context.Background(), bo)
	}()

	return nil
}

func (f *FIX) OnFIX44MarketDataRequest(msg mdr.MarketDataRequest, sID quickfix.SessionID) quickfix.MessageRejectError {
	relSym, err := msg.GetNoRelatedSym()
	if err != nil {
		return err
	}

	// Lazy shortcut
	symbol, err := relSym.Get(0).GetSymbol()
	if err != nil {
		return err
	}

	mdReqID, err := msg.GetMDReqID()
	if err != nil {
		return err
	}

	subType, err := msg.GetSubscriptionRequestType()
	if err != nil {
		return err
	}

	go func() {
		switch subType {
		default:
			rej := mdrr.New(field.NewMDReqID(mdReqID))
			quickfix.SendToTarget(rej, sID)
		case enum.SubscriptionRequestType_SNAPSHOT:
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			msg := &bitfinex.PublicSubscriptionRequest{
				Event:   "subscribe",
				Channel: bitfinex.ChanTicker,
				Symbol:  symbol,
			}

			h := func(ev interface{}) {
				// For a simple snapshot request we just need to read one message from the channel.
				go f.bfx.Websocket.Unsubscribe(context.Background(), msg)

				var data [][]float64
				switch e := ev.(type) {
				case []float64:
					return // We only care about the snapshot.
				case [][]float64:
					data = e
				}

				for _, d := range data {
					r := mdsfr.New()

					mdEntriesGroup := convert.FIX44NoMDEntriesRepeatingGroupFromTradeTicker(d)
					r.SetNoMDEntries(mdEntriesGroup)

					r.SetSymbol(symbol)
					quickfix.SendToTarget(r, sID)
				}
			}

			err := f.bfx.Websocket.Subscribe(ctx, msg, h)
			if err != nil {
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				quickfix.SendToTarget(rej, sID)
				return
			}
		case enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES:
			if _, has := f.marketDataSubscriptions[mdReqID]; has {
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				rej.SetMDReqRejReason(enum.MDReqRejReason_DUPLICATE_MDREQID)
				quickfix.SendToTarget(rej, sID)
				return
			}

			h := func(ev interface{}) {
				var data [][]float64
				switch e := ev.(type) {
				case []float64:
					return // We only care about the snapshot.
				case [][]float64:
					data = e
				}

				for _, d := range data {
					r := mdsfr.New()

					mdEntriesGroup := convert.FIX44NoMDEntriesRepeatingGroupFromTradeTicker(d)
					r.SetNoMDEntries(mdEntriesGroup)

					r.SetSymbol(symbol)
					quickfix.SendToTarget(r, sID)
				}
			}

			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			msg := &bitfinex.PublicSubscriptionRequest{
				Event:   "subscribe",
				Channel: bitfinex.ChanTicker,
				Symbol:  symbol,
			}

			err := f.bfx.Websocket.Subscribe(ctx, msg, h)
			if err != nil {
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				quickfix.SendToTarget(rej, sID)
				return
			}

			// Every new market data subscription gets a new channel that constantly
			// sends out reports.
			// XXX: How does this handle multiple market data request for the same ticker?
			f.mu.Lock()
			f.marketDataSubscriptions[mdReqID] = msg
			f.mu.Unlock()
		case enum.SubscriptionRequestType_DISABLE_PREVIOUS_SNAPSHOT_PLUS_UPDATE_REQUEST:
			if _, has := f.marketDataSubscriptions[mdReqID]; !has {
				// If we don't have a channel for the req we just ignore the disable.
				// XXX: Should we tell the other side about that?
				return
			}

			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			err := f.bfx.Websocket.Unsubscribe(ctx, f.marketDataSubscriptions[mdReqID])
			if err != nil {
				f.logger.Error("unsub", zap.Error(err))
			}
			f.mu.Lock()
			delete(f.marketDataSubscriptions, mdReqID)
			f.mu.Unlock()
		}
	}()

	return nil
}

func (f *FIX) OnFIX44OrderCancelRequest(msg ocr.OrderCancelRequest, sID quickfix.SessionID) quickfix.MessageRejectError {
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
			r.SetAccount(f.bfxUserID)
			quickfix.SendToTarget(r, sID)
			return nil
		}
		oc.ID = &idi
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
			r.SetAccount(f.bfxUserID)
			quickfix.SendToTarget(r, sID)
			return nil
		}
		oc.CID = &ocidi
		d := txnT.Format("2006-01-02")
		oc.CIDDate = &d
	}

	go func() {
		f.bfx.Websocket.Send(context.Background(), oc)
	}()

	return nil
}

func (f *FIX) OnFIX44OrderStatusRequest(msg osr.OrderStatusRequest, sID quickfix.SessionID) quickfix.MessageRejectError {
	oid, err := msg.GetOrderID()
	if err != nil {
		return err
	}

	//cid, err := msg.GetClOrdID()
	//if err != nil {
	//return err
	//}
	oidi, nerr := strconv.ParseInt(oid, 10, 64)
	order, nerr := f.bfx.Orders.Status(oidi)
	if nerr != nil {
		r := quickfix.NewBusinessMessageRejectError(nerr.Error(), 0 /*OTHER*/, nil)
		return r
	}

	er := convert.FIX44ExecutionReportFromOrder(order)
	er.SetAccount(f.bfxUserID)
	quickfix.SendToTarget(er, sID)

	return nil
}
