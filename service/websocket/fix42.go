package websocket

import (
	"github.com/bitfinexcom/bfxfixgw/convert"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
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
		p.AddOrder(clOrdID, os.Price, os.Amount, os.Symbol, p.BfxUserID(), convert.SideToFIX(t.ExecAmount))
		cached, err = p.UpdateOrder(clOrdID, orderID)
		if err != nil {
			w.logger.Warn("could not update order", zap.Error(err))
		}
	}
	totalFillQty, avgFillPx, err := p.AddExecution(orderID, execID, t.ExecPrice, t.ExecAmount)
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromTradeExecutionUpdate(t, p.BfxUserID(), cached.ClOrdID, cached.Qty, totalFillQty, avgFillPx), sID)
}

func (w *Websocket) FIX42BookSnapshot(s *bitfinex.BookUpdateSnapshot, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	var mdReqID string
	if len(s.Snapshot) > 0 {
		mdReqID = p.LookupMDReqID(s.Snapshot[0].Symbol)
	}
	quickfix.SendToTarget(convert.FIX42MarketDataFullRefreshFromBookSnapshot(mdReqID, s), sID)
}

func (w *Websocket) FIX42BookUpdate(u *bitfinex.BookUpdate, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	mdReqID := p.LookupMDReqID(u.Symbol)
	quickfix.SendToTarget(convert.FIX42MarketDataIncrementalRefreshFromBookUpdate(mdReqID, u), sID)
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
		if d.Status == "ERROR" && d.Text == "Order not found." {
			// Send out an OrderCancelReject
			orderID := strconv.FormatInt(o.ID, 10)
			clOrdID := ""
			ord, err := p.LookupByOrderID(orderID)
			if err == nil {
				clOrdID = ord.ClOrdID
			}
			quickfix.SendToTarget(convert.FIX42OrderCancelRejectFromCancel(o, p.BfxUserID(), orderID, clOrdID), sID)
			return
		} else if d.Status == "SUCCESS" {
			clOrdID := strconv.FormatInt(o.CID, 10)
			orig, err := p.LookupByClOrdID(clOrdID)
			if err != nil {
				w.logger.Error("could not reference original order to publish pending cancel execution report", zap.Error(err))
				return
			}
			quickfix.SendToTarget(convert.FIX42ExecutionReportFromCancelWithDetails(o, orig.Account, enum.ExecType_PENDING_CANCEL, orig.FilledQty(), enum.OrdStatus_PENDING_CANCEL, d.Text, orig.Symbol, orig.OrderID, orig.Side, orig.Qty, orig.AvgFillPx()), sID)
		}
		return
	case *bitfinex.OrderNew:

		order := bitfinex.Order(*o)
		var ordStatus enum.OrdStatus
		text := ""
		if d.Status == "ERROR" {
			ordStatus = enum.OrdStatus_REJECTED
			text = d.Text
		} else {
			orderID := strconv.FormatInt(o.ID, 10)
			clOrdID := strconv.FormatInt(o.CID, 10)
			// rcv server order ID
			_, err := p.UpdateOrder(clOrdID, orderID)
			if err != nil {
				w.logger.Warn("could not update order", zap.Error(err))
			}
			ordStatus = convert.OrdStatusToFIX(o.Status)
		}

		quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&order, p.BfxUserID(), enum.ExecType_PENDING_NEW, 0, ordStatus, text), sID)
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
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&ord, p.BfxUserID(), enum.ExecType_NEW, 0.0, enum.OrdStatus_NEW, ""), sID)
	return
}

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
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&ord, p.BfxUserID(), enum.ExecType_CANCELED, cached.FilledQty(), enum.OrdStatus_CANCELED, ""), sID)
	return
}
