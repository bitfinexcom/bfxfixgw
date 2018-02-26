package websocket

import (
	"github.com/bitfinexcom/bfxfixgw/convert"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	ocj "github.com/quickfixgo/fix44/ordercancelreject"
	"github.com/quickfixgo/quickfix"
	"go.uber.org/zap"
	"strconv"
)

func (w *Websocket) FIX44Handler(o interface{}, sID quickfix.SessionID) {
	w.logger.Debug("in FIX44TermDataHandler", zap.Any("object", o))
	switch d := o.(type) {
	case nil:
		return
	case *bitfinex.OrderSnapshot: // Order snapshot
		w.FIX44OrderSnapshotHandler(d, sID)
	case *bitfinex.OrderNew: // Order new
		w.FIX44OrderNewHandler(d, sID)
	case *bitfinex.OrderCancel: // Order cancel
		w.FIX44OrderCancelHandler(d, sID)
	case *bitfinex.Notification: // Notification
		w.FIX44NotificationHandler(d, sID)
	default: // unknown
		return
	}

}

func (w *Websocket) FIX44NotificationHandler(d *bitfinex.Notification, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
	}

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
			r.SetAccount(p.BfxUserID())
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

func (w *Websocket) FIX44OrderSnapshotHandler(os *bitfinex.OrderSnapshot, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
	}

	for _, o := range os.Snapshot {
		ord := bitfinex.Order(*o)
		er := convert.FIX44ExecutionReportFromOrder(&ord)
		er.SetAccount(p.BfxUserID())
		er.SetExecType(enum.ExecType_ORDER_STATUS)
		quickfix.SendToTarget(er, sID)
	}
	return
}

func (w *Websocket) FIX44OrderNewHandler(o *bitfinex.OrderNew, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
	}

	ord := bitfinex.Order(*o)
	er := convert.FIX44ExecutionReportFromOrder(&ord)
	er.SetAccount(p.BfxUserID())
	quickfix.SendToTarget(er, sID)
	return
}

func (w *Websocket) FIX44OrderCancelHandler(o *bitfinex.OrderCancel, sID quickfix.SessionID) {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
	}

	ord := bitfinex.Order(*o)
	er := convert.FIX44ExecutionReportFromOrder(&ord)
	er.SetExecType(enum.ExecType_CANCELED)
	er.SetAccount(p.BfxUserID())
	quickfix.SendToTarget(er, sID)
	return
}
