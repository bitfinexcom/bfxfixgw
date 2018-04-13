package websocket

import (
	"github.com/bitfinexcom/bfxfixgw/convert"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/fix42/logout"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
	"strconv"
)

// Handle Bitfinex messages and process them as FIX42 downstream.

// FIX42Handler processes websocket -> FIX
/*
func (w *Websocket) FIX42Handler(o interface{}, sID quickfix.SessionID) {
	w.logger.Debug("in FIX42TermDataHandler", zap.Any("object", o))

	switch d := o.(type) {
	case *bitfinex.OrderSnapshot: // Order snapshot
		w.FIX42OrderSnapshotHandler(d, sID)
	case *bitfinex.OrderNew: // Order new
		w.FIX42OrderNewHandler(d, sID)
	case *bitfinex.OrderCancel: // Order cancel
		w.FIX42OrderCancelHandler(d, sID)
	case *bitfinex.Notification: // Notification
		w.FIX42NotificationHandler(d, sID)
	default: // unknown
		return
	}
}
*/

func (w *Websocket) FIX42HandleAuth(auth *websocket.AuthEvent, sID quickfix.SessionID) {
	if auth.Status == "FAILED" {
		logout := logout.New()
		logout.SetText(auth.Message)
		quickfix.SendToTarget(logout, sID)
	}
}

// public trades
func (w *Websocket) FIX42TradeHandler(t *bitfinex.Trade, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	if reqID, ok := p.LookupMDReqID(t.Pair); ok {
		fix := convert.FIX42MarketDataIncrementalRefreshFromTrade(reqID, t)
		quickfix.SendToTarget(fix, sID)
	} else {
		w.logger.Warn("could not find MDReqID for BFX trade", zap.String("Pair", t.Pair))
	}
}

func (w *Websocket) FIX42TradeSnapshotHandler(s *bitfinex.TradeSnapshot, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	if len(s.Snapshot) > 0 {
		t := s.Snapshot[0]
		if reqID, ok := p.LookupMDReqID(t.Pair); ok {
			fix := convert.FIX42MarketDataFullRefreshFromTradeSnapshot(reqID, s, w.Symbology, sID.SenderCompID)
			quickfix.SendToTarget(fix, sID)
		} else {
			w.logger.Warn("could not find MDReqID for BFX trade", zap.String("Pair", t.Pair))
			return
		}
	} // else no-op
}

func (w *Websocket) FIX42TradeExecutionUpdateHandler(t *bitfinex.TradeExecutionUpdate, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	orderID := strconv.FormatInt(t.OrderID, 10)
	execID := strconv.FormatInt(t.ID, 10)
	cached, err := p.LookupByOrderID(orderID)
	// can't find order
	if err != nil {
		// try a REST fetch
		w.logger.Warn("order not in cache, falling back to REST", zap.String("OrderID", orderID))
		os, err2 := p.Rest.Orders.Status(t.OrderID)
		if err2 != nil {
			// couldn't fallback to REST
			w.logger.Error("could not process trade execution update", zap.Error(err), zap.Error(err2))
			return
		}
		w.logger.Info("fetch order info from REST: OK", zap.String("OrderID", orderID))
		orderID := strconv.FormatInt(os.ID, 10)
		clOrdID := strconv.FormatInt(os.CID, 10)
		// update everything at the same time
		p.AddOrder(clOrdID, os.Price, os.Amount, os.Symbol, p.BfxUserID(), convert.SideToFIX(t.ExecAmount), convert.OrdTypeToFIX(os.Type))
		cached, err = p.UpdateOrder(clOrdID, orderID)
		if err != nil {
			w.logger.Warn("could not update order", zap.Error(err))
		}
	}
	totalFillQty, avgFillPx, err := p.AddExecution(orderID, execID, t.ExecPrice, t.ExecAmount)
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromTradeExecutionUpdate(t, p.BfxUserID(), cached.ClOrdID, cached.Qty, totalFillQty, avgFillPx, w.Symbology, sID.SenderCompID), sID)
}

func (w *Websocket) FIX42BookSnapshot(s *bitfinex.BookUpdateSnapshot, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	var mdReqID string
	if len(s.Snapshot) > 0 {
		mdReqID, ok = p.LookupMDReqID(s.Snapshot[0].Symbol)
		if ok {
			quickfix.SendToTarget(convert.FIX42MarketDataFullRefreshFromBookSnapshot(mdReqID, s, w.Symbology, sID.SenderCompID), sID)
		} else {
			w.logger.Warn("could not find MDReqID for symbol", zap.String("MDReqID", mdReqID))
		}
	}

}

func (w *Websocket) FIX42BookUpdate(u *bitfinex.BookUpdate, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	mdReqID, ok := p.LookupMDReqID(u.Symbol)
	if ok {
		quickfix.SendToTarget(convert.FIX42MarketDataIncrementalRefreshFromBookUpdate(mdReqID, u), sID)
	} else {
		w.logger.Warn("could not find MDReqID for symbol", zap.String("MDReqID", mdReqID))
	}
}

func (w *Websocket) FIX42NotificationHandler(d *bitfinex.Notification, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	switch o := d.NotifyInfo.(type) {
	case *bitfinex.OrderCancel:
		// Only handling error currently.
		if d.Status == "ERROR" {
			// Send out an OrderCancelReject
			// BFX API returns only the original ClOrdID, not the cancel ClOrdID in acknowledgements.
			// Must reference cache mapping to obtain cancel's ClOrdID
			orderID := strconv.FormatInt(o.ID, 10)
			origClOrdID := strconv.FormatInt(o.CID, 10)
			cxlClOrdID := origClOrdID // error case :(
			cache, err := p.LookupCancelByOrigClOrdID(origClOrdID)
			if err == nil {
				cxlClOrdID = cache.ClOrdID
			}
			quickfix.SendToTarget(convert.FIX42OrderCancelRejectFromCancel(o, p.BfxUserID(), orderID, origClOrdID, cxlClOrdID, d.Text), sID)
			return
		} else if d.Status == "SUCCESS" {
			clOrdID := strconv.FormatInt(o.CID, 10)
			orig, err := p.LookupByClOrdID(clOrdID)
			if err != nil {
				w.logger.Error("could not reference original order to publish pending cancel execution report", zap.Error(err))
				return
			}
			quickfix.SendToTarget(convert.FIX42ExecutionReportFromCancelWithDetails(o, orig.Account, enum.ExecType_PENDING_CANCEL, orig.FilledQty(), enum.OrdStatus_PENDING_CANCEL, orig.OrderType, d.Text, orig.Symbol, orig.ClOrdID, orig.OrderID, orig.Side, orig.Qty, orig.AvgFillPx(), w.Symbology, sID.SenderCompID), sID)
		}
		return
	case *bitfinex.OrderNew:

		order := bitfinex.Order(*o)
		var ordStatus enum.OrdStatus
		var execType enum.ExecType
		text := ""
		if d.Status == "ERROR" {
			ordStatus = enum.OrdStatus_REJECTED
			execType = enum.ExecType_REJECTED
			text = d.Text
		} else {
			orderID := strconv.FormatInt(o.ID, 10)
			clOrdID := strconv.FormatInt(o.CID, 10)
			// rcv server order ID
			_, err := p.UpdateOrder(clOrdID, orderID)
			if err != nil {
				w.logger.Warn("adding unknown order (entered outside session)", zap.String("ClOrdID", clOrdID), zap.String("OrderID", orderID))
				cache := p.AddOrder(clOrdID, order.Price, order.Amount, order.Symbol, p.BfxUserID(), convert.SideToFIX(order.Amount), convert.OrdTypeToFIX(order.Type))
				cache.OrderID = orderID
			}
			ordStatus = enum.OrdStatus_PENDING_NEW
			execType = enum.ExecType_PENDING_NEW
		}
		quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&order, p.BfxUserID(), execType, 0, ordStatus, text, w.Symbology, sID.SenderCompID), sID)
		// market order ack
		if (o.Type == bitfinex.OrderTypeMarket || o.Type == bitfinex.OrderTypeExchangeMarket) && (o.Status == "SUCCESS" || o.Status == "") {
			// synthetically publish a followup NEW
			quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&order, p.BfxUserID(), enum.ExecType_NEW, 0, enum.OrdStatus_NEW, text, w.Symbology, sID.SenderCompID), sID)
		}

		return
		// TODO other types
	default:
		w.logger.Warn("unhandled notify info object", zap.Any("msg", d.NotifyInfo))
		return
	}
}

func (w *Websocket) FIX42OrderSnapshotHandler(os *bitfinex.OrderSnapshot, sID quickfix.SessionID) {
	_, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	/*
		TODO
		for _, o := range os.Snapshot {
			quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(o, p.BfxUserID(), enum.ExecType_ORDER_STATUS), sID)
		}
	*/
	return
}

// for working orders after notification 'ack'
func (w *Websocket) FIX42OrderNewHandler(o *bitfinex.OrderNew, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	ord := bitfinex.Order(*o)
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&ord, p.BfxUserID(), enum.ExecType_NEW, 0.0, enum.OrdStatus_NEW, "", w.Symbology, sID.SenderCompID), sID)
	return
}

// for working orders after notification 'ack'
func (w *Websocket) FIX42OrderUpdateHandler(o *bitfinex.OrderUpdate, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	ord := bitfinex.Order(*o)
	ordStatus := convert.OrdStatusToFIX(o.Status)
	execType := convert.ExecTypeToFIX(o.Status)
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&ord, p.BfxUserID(), execType, 0.0, ordStatus, "", w.Symbology, sID.SenderCompID), sID)
	return
}

//[0,"oc",[1149698616,null,57103053041,"tBTCUSD",1523634703091,1523634703127,0,0.1,"EXCHANGE LIMIT",null,null,null,0,"EXECUTED @ 1662.9(0.05): was PARTIALLY FILLED @ 1661.5(0.05)",null,null,1670,1662.2,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]
func (w *Websocket) FIX42OrderCancelHandler(o *bitfinex.OrderCancel, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	ord := bitfinex.Order(*o)
	orderID := strconv.FormatInt(o.ID, 10)
	cached, err := p.LookupByOrderID(orderID)
	// add cancel related to this order
	if err != nil {
		w.logger.Error("could not reference original order to publish cancel ack execution report", zap.Error(err))
		return
	}
	// oc is simply a terminal state for an order, may be a full fill here
	execType := convert.ExecTypeToFIX(ord.Status)
	ordStatus := convert.OrdStatusToFIX(ord.Status)
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&ord, p.BfxUserID(), execType, cached.FilledQty(), ordStatus, string(ord.Status), w.Symbology, sID.SenderCompID), sID)
	return
}
