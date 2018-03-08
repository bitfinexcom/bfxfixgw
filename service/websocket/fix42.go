package websocket

import (
	"github.com/bitfinexcom/bfxfixgw/convert"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
)

// Handle Bitfinex messages and process them as FIX42 downstream.

// FIX42Handler processes websocket -> FIX
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

// 6 = AvgPx
// 14 = CumQty
// 17 = ExecID
// 20 = ExecTransType
// 37 = OrderID
// 39 = OrdStatus
// 54 = Side
// 55 = Symbol
// 150 = ExecType
// 151 = LeavesQty
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
			quickfix.SendToTarget(convert.FIX42OrderCancelRejectFromCancel(o, p.BfxUserID()), sID)
			return
		}
		return
	case *bitfinex.OrderNew:
		order := bitfinex.Order(*o)
		quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&order, "acct", enum.ExecType_NEW), sID)
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

func (w *Websocket) FIX42OrderNewHandler(o *bitfinex.OrderNew, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	ord := bitfinex.Order(*o)
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&ord, p.BfxUserID(), enum.ExecType_ORDER_STATUS), sID)
	return
}

func (w *Websocket) FIX42OrderCancelHandler(o *bitfinex.OrderCancel, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return
	}
	ord := bitfinex.Order(*o)
	quickfix.SendToTarget(convert.FIX42ExecutionReportFromOrder(&ord, p.BfxUserID(), enum.ExecType_CANCELED), sID)
	return
}
