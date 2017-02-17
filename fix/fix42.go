package fix

import (
	"strconv"

	"github.com/bitfinexcom/bfxfixgw/convert"

	"github.com/knarz/bitfinex-api-go"
	"github.com/quickfixgo/quickfix"
	"github.com/uber-go/zap"

	//er "github.com/quickfixgo/quickfix/fix42/executionreport"
	mdr "github.com/quickfixgo/quickfix/fix42/marketdatarequest"
	mdrr "github.com/quickfixgo/quickfix/fix42/marketdatarequestreject"
	mdsfr "github.com/quickfixgo/quickfix/fix42/marketdatasnapshotfullrefresh"
	nos "github.com/quickfixgo/quickfix/fix42/newordersingle"
	ocj "github.com/quickfixgo/quickfix/fix42/ordercancelreject"
	ocr "github.com/quickfixgo/quickfix/fix42/ordercancelrequest"
	osr "github.com/quickfixgo/quickfix/fix42/orderstatusrequest"

	"github.com/quickfixgo/quickfix/enum"
	"github.com/quickfixgo/quickfix/field"
)

func (f *FIX) FIX42TermDataHandler(d bitfinex.TermData, sID quickfix.SessionID) {
	if d.Term == "hb" || d.Term == "hos" || d.Term == "miu" || d.Term == "fiu" || d.Term == "fis" || d.Term == "wu" || d.Term == "bu" || d.Term == "mis" {
		return
	}
	f.logger.Debug("in FIX42TermDataHandler", zap.Object("termData", d))

	if d.HasError() {
		return
	}

	switch d.Term {
	case "os": // Order status
		f.FIX42TermDataOrderStatusHandler(d, sID)
	case "on": // Order new
		f.FIX42TermDataOrderNewHandler(d, sID)
	case "oc": // Order cancel
		f.FIX42TermDataOrderCancelHandler(d, sID)
	case "n": // Order cancel
		f.FIX42TermDataNotificationHandler(d, sID)
	default: // unknown
		return
	}
}

func (f *FIX) FIX42TermDataNotificationHandler(d bitfinex.TermData, sID quickfix.SessionID) {
	if len(d.Data) < 8 {
		return
	}

	//ts := d.Data[0]
	reason := d.Data[1]
	//msgID := d.Data[2]
	//? := d.Data[3]
	// code := d.Data[5]
	status := d.Data[6]
	msg := d.Data[7]
	switch reason {
	case "oc-req":
		o, err := convert.OrderFromTermData(d.Data[4].([]interface{})) // This should be an order object
		if err != nil {
			return
		}

		// Only handling error currently.
		if status == "ERROR" && msg == "Order not found." {
			// Send out an OrderCancelReject
			r := ocj.New(
				field.NewOrderID("NONE"),
				field.NewClOrdID("NONE"), // XXX: This should be the actual ClOrdID which we don't have in this context.
				field.NewOrigClOrdID(strconv.FormatInt(o.CID, 10)),
				field.NewOrdStatus(enum.OrdStatus_REJECTED),
				field.NewCxlRejResponseTo(enum.CxlRejResponseTo_ORDER_CANCEL_REQUEST),
			)
			r.SetCxlRejReason(enum.CxlRejReason_UNKNOWN_ORDER)
			r.SetAccount(strconv.FormatInt(f.bfxUserIDs[sID], 10))
			quickfix.SendToTarget(r, sID)
			return
		}
		return
	case "on-req":
		// XXX: Handle this at some point.
		return
	default:
		return
	}
}

func (f *FIX) FIX42TermDataOrderStatusHandler(d bitfinex.TermData, sID quickfix.SessionID) {
	o, err := convert.OrderFromTermData(d.Data)
	if err != nil {
		return // Skip order. XXX: Is there a better way?
	}

	er := convert.FIX42ExecutionReportFromWebsocketV2Order(o)
	er.SetAccount(strconv.FormatInt(f.bfxUserIDs[sID], 10))
	er.SetExecType(enum.ExecType_ORDER_STATUS)
	return
}

func (f *FIX) FIX42TermDataOrderNewHandler(d bitfinex.TermData, sID quickfix.SessionID) {
	o, err := convert.OrderFromTermData(d.Data)
	if err != nil {
		return
	}

	er := convert.FIX42ExecutionReportFromWebsocketV2Order(o)
	er.SetAccount(strconv.FormatInt(f.bfxUserIDs[sID], 10))
	quickfix.SendToTarget(er, sID)
	return
}

func (f *FIX) FIX42TermDataOrderCancelHandler(d bitfinex.TermData, sID quickfix.SessionID) {
	o, err := convert.OrderFromTermData(d.Data)
	if err != nil {
		return
	}

	er := convert.FIX42ExecutionReportFromWebsocketV2Order(o)
	er.SetExecType(enum.ExecType_CANCELED)
	er.SetAccount(strconv.FormatInt(f.bfxUserIDs[sID], 10))
	quickfix.SendToTarget(er, sID)
	return
}

func (f *FIX) OnFIX42NewOrderSingle(msg nos.NewOrderSingle, sID quickfix.SessionID) quickfix.MessageRejectError {
	bo, err := convert.WebSocketV2OrderNewFromFIX42NewOrderSingle(msg)
	if err != nil {
		return err
	}

	go func() {
		// XXX: handle error?
		f.bfx.WebSocket.SendPrivate(bo)
	}()

	return nil
}

func (f *FIX) OnFIX42MarketDataRequest(msg mdr.MarketDataRequest, sID quickfix.SessionID) quickfix.MessageRejectError {
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

	// XXX: The following could most likely be abtracted to work both for 4.2 and 4.4.
	go func() {
		switch subType {
		default:
			rej := mdrr.New(field.NewMDReqID(mdReqID))
			quickfix.SendToTarget(rej, sID)
		case enum.SubscriptionRequestType_SNAPSHOT:
			dc := NewMarketDataChan()

			err := f.bfx.WebSocket.Subscribe(bitfinex.CHAN_TICKER, symbol, dc.C)
			if err != nil {
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				quickfix.SendToTarget(rej, sID)
				return
			}

			// For a simple snapshot request we just need to read one message from the channel.
			go func() {
				data := <-dc.Receive()
				err = f.bfx.WebSocket.UnsubscribeByChannel(dc.C)
				if err != nil {
					f.logger.Error("unsub", zap.Error(err))
				}
				defer dc.Close(err)

				r := mdsfr.New(field.NewSymbol(symbol))

				mdEntriesGroup := convert.FIX42NoMDEntriesRepeatingGroupFromTradeTicker(data)
				r.SetNoMDEntries(mdEntriesGroup)

				r.SetSymbol(symbol)
				quickfix.SendToTarget(r, sID)
			}()
		case enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES:
			if _, has := f.marketDataChans[mdReqID]; has {
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				rej.SetMDReqRejReason(enum.MDReqRejReason_DUPLICATE_MDREQID)
				quickfix.SendToTarget(rej, sID)
				return
			}
			// Every new market data subscription gets a new channel that constantly
			// sends out reports.
			// XXX: How does this handle multiple market data request for the same ticker?
			f.mu.Lock()
			f.marketDataChans[mdReqID] = NewMarketDataChan()
			f.mu.Unlock()

			err := f.bfx.WebSocket.Subscribe(bitfinex.CHAN_TICKER, symbol, f.marketDataChans[mdReqID].C)
			if err != nil {
				rej := mdrr.New(field.NewMDReqID(mdReqID))
				quickfix.SendToTarget(rej, sID)
				return
			}

			go func() {
				for {
					select {
					case data := <-f.marketDataChans[mdReqID].Receive():
						r := mdsfr.New(field.NewSymbol(symbol))

						mdEntriesGroup := convert.FIX42NoMDEntriesRepeatingGroupFromTradeTicker(data)
						r.SetNoMDEntries(mdEntriesGroup)

						quickfix.SendToTarget(r, sID)
					case <-f.marketDataChans[mdReqID].Done():
						return
					}
				}
			}()
		case enum.SubscriptionRequestType_DISABLE_PREVIOUS_SNAPSHOT_PLUS_UPDATE_REQUEST:
			if _, has := f.marketDataChans[mdReqID]; !has {
				// If we don't have a channel for the req we just ignore the disable.
				// XXX: Should we tell the other side about that?
				return
			}

			err := f.bfx.WebSocket.UnsubscribeByChannel(f.marketDataChans[mdReqID].C)
			if err != nil {
				f.logger.Error("unsub", zap.Error(err))
			}
			defer f.marketDataChans[mdReqID].Close(nil)
			defer delete(f.marketDataChans, mdReqID)
		}
	}()

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

	oc := &bitfinex.WebSocketV2OrderCancel{}

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
			r.SetAccount(strconv.FormatInt(f.bfxUserIDs[sID], 10))
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
			r.SetAccount(strconv.FormatInt(f.bfxUserIDs[sID], 10))
			quickfix.SendToTarget(r, sID)
			return nil
		}
		oc.CID = &ocidi
		d := txnT.Format("2006-01-02")
		oc.CIDDate = &d
	}

	go func() {
		f.bfx.WebSocket.SendPrivate(oc)
	}()

	return nil
}

func (f *FIX) OnFIX42OrderStatusRequest(msg osr.OrderStatusRequest, sID quickfix.SessionID) quickfix.MessageRejectError {
	oid, err := msg.GetOrderID()
	if err != nil {
		return err
	}

	//cid, err := msg.GetClOrdID()
	//if err != nil {
	//return err
	//}

	oidi, nerr := strconv.ParseInt(oid, 10, 64)
	o, nerr := f.bfxV1.Orders.Status(oidi)
	if nerr != nil {
		r := quickfix.NewBusinessMessageRejectError(nerr.Error(), 0 /*OTHER*/, nil)
		return r
	}

	er := convert.FIX42ExecutionReportFromOrder(&o)
	er.SetAccount(strconv.FormatInt(f.bfxUserIDs[sID], 10))
	quickfix.SendToTarget(er, sID)

	return nil
}
