package websocket

import (
	"errors"
	"github.com/quickfixgo/field"
	"strconv"

	"github.com/bitfinexcom/bfxfixgw/convert"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/bitfinexcom/bitfinex-api-go/v2/websocket"
	"github.com/quickfixgo/enum"
	lgout42 "github.com/quickfixgo/fix42/logout"
	lgout44 "github.com/quickfixgo/fix44/logout"
	lgoutfixt "github.com/quickfixgo/fixt11/logout"
	"github.com/quickfixgo/quickfix"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"strings"
)

// Handle Bitfinex messages and process them as FIX downstream.

// FIXHandler processes websocket -> FIX
/*
func (w *Websocket) FIXHandler(o interface{}, sID quickfix.SessionID) {
	w.logger.Debug("in FIXTermDataHandler", zap.Any("object", o))

	switch d := o.(type) {
	case *bitfinex.OrderSnapshot: // Order snapshot
		w.FIXOrderSnapshotHandler(d, sID)
	case *bitfinex.OrderNew: // Order new
		w.FIXOrderNewHandler(d, sID)
	case *bitfinex.OrderCancel: // Order cancel
		w.FIXOrderCancelHandler(d, sID)
	case *bitfinex.Notification: // Notification
		w.FIXNotificationHandler(d, sID)
	default: // unknown
		return
	}
}
*/

// FIXHandleAuth handles a websocket auth event
func (w *Websocket) FIXHandleAuth(auth *websocket.AuthEvent, sID quickfix.SessionID) error {
	if auth.Status == "FAILED" {
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
		msg.Set(field.NewText(auth.Message))
		return quickfix.SendToTarget(msg, sID)
	}
	return nil
}

// FIXTradeHandler handles public trades
func (w *Websocket) FIXTradeHandler(t *bitfinex.Trade, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}
	if reqID, ok := p.LookupMDReqID(t.Pair); ok {
		fix := convert.FIXMarketDataIncrementalRefreshFromTrade(sID.BeginString, reqID, t, w.Symbology, sID.TargetCompID)
		return quickfix.SendToTarget(fix, sID)
	} else {
		w.logger.Warn("could not find MDReqID for BFX trade", zap.String("Pair", t.Pair))
	}
	return nil
}

// FIXTradeSnapshotHandler handles trade snapshots
func (w *Websocket) FIXTradeSnapshotHandler(s *bitfinex.TradeSnapshot, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}
	if len(s.Snapshot) > 0 {
		t := s.Snapshot[0]
		if reqID, ok := p.LookupMDReqID(t.Pair); ok {
			fix := convert.FIXMarketDataFullRefreshFromTradeSnapshot(sID.BeginString, reqID, s, w.Symbology, sID.TargetCompID)
			return quickfix.SendToTarget(fix, sID)
		} else {
			w.logger.Warn("could not find MDReqID for BFX trade", zap.String("Pair", t.Pair))
			return nil
		}
	} // else no-op
	return nil
}

// FIXTradeExecutionUpdateHandler handles trade snapshots
func (w *Websocket) FIXTradeExecutionUpdateHandler(t *bitfinex.TradeExecutionUpdate, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}
	orderID := strconv.FormatInt(t.OrderID, 10)
	execID := strconv.FormatInt(t.ID, 10)
	cached, err := p.LookupByOrderID(orderID)
	// can't find order
	if err != nil {
		// try a REST fetch
		w.logger.Warn("order not in cache, falling back to REST", zap.String("OrderID", orderID))
		os, err2 := p.Rest.Orders.GetByOrderId(t.OrderID)
		if err2 != nil {
			// couldn't fallback to REST
			w.logger.Error("could not process trade execution", zap.Error(err), zap.Error(err2))
			return nil
		}
		w.logger.Info("fetch order info from REST: OK", zap.String("OrderID", orderID))
		orderID := strconv.FormatInt(os.ID, 10)
		clOrdID := strconv.FormatInt(os.CID, 10)
		// update everything at the same time
		ordtype := bitfinex.OrderType(os.Type)
		tif, _ := convert.TimeInForceToFIX(ordtype, os.MTSTif)
		ot, isMargin := convert.OrdTypeToFIX(ordtype)
		p.AddOrder(clOrdID, os.Price, os.PriceAuxLimit, os.PriceTrailing, os.Amount, os.Symbol, p.BfxUserID(), convert.SideToFIX(t.ExecAmount), ot, isMargin, tif, os.MTSTif, int(os.Flags))
		cached, err = p.UpdateOrder(clOrdID, orderID)
		if err != nil {
			w.logger.Warn("could not update order", zap.Error(err))
		}
	}
	totalFillQty, avgFillPx, err := p.AddExecution(orderID, execID, t.ExecPrice, t.ExecAmount)
	if err != nil {
		return err
	}
	return quickfix.SendToTarget(convert.FIXExecutionReportFromTradeExecutionUpdate(sID.BeginString, t, p.BfxUserID(), cached.ClOrdID, cached.Qty, totalFillQty, cached.Px, cached.Stop, cached.Trail, avgFillPx, w.Symbology, sID.TargetCompID, cached.TifExpiration, cached.Flags), sID)
}

// FIXBookSnapshot handles a book update snapshot
func (w *Websocket) FIXBookSnapshot(s *bitfinex.BookUpdateSnapshot, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}
	var mdReqID string
	if len(s.Snapshot) > 0 {
		mdReqID, ok = p.LookupMDReqID(s.Snapshot[0].Symbol)
		if ok {
			return quickfix.SendToTarget(convert.FIXMarketDataFullRefreshFromBookSnapshot(sID.BeginString, mdReqID, s, w.Symbology, sID.TargetCompID), sID)
		} else {
			w.logger.Warn("could not find MDReqID for symbol", zap.String("MDReqID", mdReqID))
		}
	}
	return nil
}

// FIXBookUpdate handles a book update
func (w *Websocket) FIXBookUpdate(u *bitfinex.BookUpdate, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}
	mdReqID, ok := p.LookupMDReqID(u.Symbol)
	if ok {
		return quickfix.SendToTarget(convert.FIXMarketDataIncrementalRefreshFromBookUpdate(sID.BeginString, mdReqID, u, w.Symbology, sID.TargetCompID), sID)
	} else {
		w.logger.Warn("could not find MDReqID for symbol", zap.String("MDReqID", mdReqID))
	}
	return nil
}

// FIXNotificationHandler handles a bitfinex notification
func (w *Websocket) FIXNotificationHandler(d *bitfinex.Notification, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
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
			return quickfix.SendToTarget(convert.FIXOrderCancelReject(sID.BeginString, p.BfxUserID(), orderID, origClOrdID, cxlClOrdID, d.Text, false), sID)
		} else if d.Status == "SUCCESS" {
			clOrdID := strconv.FormatInt(o.CID, 10)
			orig, err := p.LookupByClOrdID(clOrdID)
			if err != nil {
				w.logger.Error("could not reference original order to publish pending cancel execution report", zap.Error(err))
				return err
			}

			exp, _ := convert.MTSToTime(orig.TifExpiration)
			er := convert.FIXExecutionReport(sID.BeginString, orig.Symbol, orig.ClOrdID, orig.OrderID, orig.Account, enum.ExecType_PENDING_CANCEL, orig.Side, orig.Qty, 0.0, orig.FilledQty(), orig.Px, orig.Stop, orig.Trail, orig.AvgFillPx(), enum.OrdStatus_PENDING_CANCEL, orig.OrderType, orig.IsMargin, orig.TimeInForce, exp, d.Text, w.Symbology, sID.TargetCompID, orig.Flags)
			if orig.Px > 0 {
				er.Set(field.NewPrice(decimal.NewFromFloat(orig.Px), 4))
			}
			return quickfix.SendToTarget(er, sID)
		}
		return nil
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
				ordtype := bitfinex.OrderType(order.Type)
				tif, _ := convert.TimeInForceToFIX(ordtype, order.MTSTif)
				ot, isMargin := convert.OrdTypeToFIX(ordtype)
				cache := p.AddOrder(clOrdID, order.Price, order.PriceAuxLimit, order.PriceTrailing, order.Amount, order.Symbol, p.BfxUserID(), convert.SideToFIX(order.Amount), ot, isMargin, tif, order.MTSTif, int(order.Flags))
				cache.OrderID = orderID
			}
			ordStatus = enum.OrdStatus_NEW
			execType = enum.ExecType_NEW
		}
		// oddly order new acks don't include order flags, so we can reference the original order to include these flags
		flags := int(o.Flags) // always empty
		orig, err := p.LookupByClOrdID(strconv.FormatInt(order.CID, 10))
		if err == nil {
			flags = orig.Flags
		}
		// notification ack doesn't include the peg price, but the price the order is currently sitting at (goes into 99 StopPx)
		peg := 0.0
		stop := o.PriceAuxLimit
		if strings.Contains(o.Type, "TRAILING") {
			// ref original order
			orig, err := p.LookupByClOrdID(strconv.FormatInt(o.CID, 10))
			if err == nil {
				peg = orig.Trail
			}
			stop = o.Price
		}
		return quickfix.SendToTarget(convert.FIXExecutionReportFromOrder(sID.BeginString, &order, p.BfxUserID(), execType, 0, ordStatus, text, w.Symbology, sID.TargetCompID, flags, stop, peg), sID)
	default:
		w.logger.Warn("unhandled notify info object", zap.Any("msg", d.NotifyInfo))
	}
	return nil
}

// FIXOrderSnapshotHandler handles an incoming order snapshot
func (w *Websocket) FIXOrderSnapshotHandler(os *bitfinex.OrderSnapshot, sID quickfix.SessionID) error {
	peer, ok := w.FindPeer(sID.String())
	if ok {
		for _, order := range os.Snapshot {
			ordtype := bitfinex.OrderType(order.Type)
			tif, _ := convert.TimeInForceToFIX(ordtype, order.MTSTif)

			// add order to cache
			ot, isMargin := convert.OrdTypeToFIX(ordtype)
			cache := peer.AddOrder(strconv.FormatInt(order.CID, 10), order.Price, order.PriceAuxLimit, order.PriceTrailing, order.Amount, order.Symbol, peer.BfxUserID(), convert.SideToFIX(order.Amount), ot, isMargin, tif, order.MTSTif, int(order.Flags))

			// need to fetch executions for this order to fill cache execution details
			snapshot, err := peer.Rest.Orders.OrderTrades(order.Symbol, order.ID)
			if err != nil {
				w.logger.Warn("could not find executions for open order", zap.Int64("OrderID", order.ID), zap.Error(err))
				continue
			}
			if snapshot == nil {
				w.logger.Info("empty order trade snapshot", zap.Int64("OrderID", order.ID))
			} else {
				for _, tu := range snapshot.Snapshot {
					ordid := strconv.FormatInt(tu.OrderID, 10)
					execid := strconv.FormatInt(tu.ID, 10)
					if _, _, err := peer.AddExecution(ordid, execid, tu.ExecPrice, tu.ExecAmount); err != nil {
						return err
					}
					w.logger.Info("mapped execution to working order", zap.String("OrderID", ordid), zap.String("ExecID", execid))
				}
			}
			cache.OrderID = strconv.FormatInt(order.ID, 10)
			er := convert.FIXExecutionReportFromOrder(sID.BeginString, order, peer.BfxUserID(), enum.ExecType_NEW, cache.FilledQty(), enum.OrdStatus_NEW, string(order.Status), w.Symbology, sID.TargetCompID, int(order.Flags), order.PriceAuxLimit, order.PriceTrailing)
			er.Set(field.NewAvgPx(decimal.NewFromFloat(cache.AvgFillPx()), 4))
			return quickfix.SendToTarget(er, sID)
		}
	}
	return nil
}

// FIXOrderNewHandler is for working orders after notification 'ack'
func (w *Websocket) FIXOrderNewHandler(o *bitfinex.OrderNew, sID quickfix.SessionID) error {
	// order new notification is sent prior to this message and is translated into a NEW ER.
	// this message is received is a limit order is resting on the book after submission,
	// but the corresponding execution report has already been sent (server did not reject)

	// no-op.
	return nil
}

// FIXOrderUpdateHandler is for working orders after notification 'ack'
func (w *Websocket) FIXOrderUpdateHandler(o *bitfinex.OrderUpdate, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}
	ord := bitfinex.Order(*o)
	ordStatus := convert.OrdStatusToFIX(o.Status)
	execType := convert.ExecTypeToFIX(o.Status)
	peg := o.PriceTrailing
	stop := o.PriceAuxLimit
	if strings.Contains(ord.Type, "TRAILING") && peg <= 0 {
		// attempt to lookup peg
		cache, err := p.LookupByClOrdID(strconv.FormatInt(ord.CID, 10))
		if err == nil {
			peg = cache.Trail
		}
	}
	return quickfix.SendToTarget(convert.FIXExecutionReportFromOrder(sID.BeginString, &ord, p.BfxUserID(), execType, 0.0, ordStatus, "", w.Symbology, sID.TargetCompID, int(o.Flags), stop, peg), sID)
}

//FIXOrderCancelHandler handles order cancels
//[0,"oc",[1149698616,null,57103053041,"tBTCUSD",1523634703091,1523634703127,0,0.1,"EXCHANGE LIMIT",null,null,null,0,"EXECUTED @ 1662.9(0.05): was PARTIALLY FILLED @ 1661.5(0.05)",null,null,1670,1662.2,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]
func (w *Websocket) FIXOrderCancelHandler(o *bitfinex.OrderCancel, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}
	ord := bitfinex.Order(*o)
	orderID := strconv.FormatInt(o.ID, 10)
	cached, err := p.LookupByOrderID(orderID)
	// add cancel related to this order
	if err != nil {
		w.logger.Error("could not reference original order to publish cancel ack execution report", zap.Error(err))
		return err
	}
	// oc is simply a terminal state for an order, may be a full fill here
	execType := convert.ExecTypeToFIX(ord.Status)
	ordStatus := convert.OrdStatusToFIX(ord.Status)
	if ordStatus == enum.OrdStatus_FILLED || ordStatus == enum.OrdStatus_PARTIALLY_FILLED {
		return nil // do not publish duplicate execution report--tu/te will have more information (fees, etc.) for this event
	}
	return quickfix.SendToTarget(convert.FIXExecutionReportFromOrder(sID.BeginString, &ord, p.BfxUserID(), execType, cached.FilledQty(), ordStatus, string(ord.Status), w.Symbology, sID.TargetCompID, cached.Flags, 0.0, cached.Trail), sID)
}

// FIXWalletUpdateHandler is for wallet updates
func (w *Websocket) FIXWalletUpdateHandler(u *bitfinex.WalletUpdate, sID quickfix.SessionID) error {
	wallet := bitfinex.Wallet(*u)
	snap := bitfinex.WalletSnapshot{Snapshot: []*bitfinex.Wallet{&wallet}}
	return w.FIXWalletSnapshotHandler(&snap, sID)
}

// FIXWalletSnapshotHandler is for wallet snapshots
func (w *Websocket) FIXWalletSnapshotHandler(s *bitfinex.WalletSnapshot, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}

	for _, wallet := range s.Snapshot {
		posRep := convert.FIXPositionReportFromWallet(sID.BeginString, wallet, p.BfxUserID())
		if err := quickfix.SendToTarget(posRep, sID); err != nil {
			return err
		}
	}

	return nil
}

// FIXPositionUpdateHandler is for wallet updates
func (w *Websocket) FIXPositionUpdateHandler(u *bitfinex.PositionUpdate, sID quickfix.SessionID) error {
	position := bitfinex.Position(*u)
	snap := bitfinex.PositionSnapshot{Snapshot: []*bitfinex.Position{&position}}
	return w.FIXPositionSnapshotHandler(&snap, sID)
}

// FIXPositionSnapshotHandler is for wallet snapshots
func (w *Websocket) FIXPositionSnapshotHandler(s *bitfinex.PositionSnapshot, sID quickfix.SessionID) error {
	p, ok := w.FindPeer(sID.String())
	if !ok {
		w.logger.Warn("could not find peer for SessionID", zap.String("SessionID", sID.String()))
		return nil
	}

	for _, position := range s.Snapshot {
		posRep := convert.FIXPositionReportFromPosition(sID.BeginString, position, p.BfxUserID(), w.Symbology, sID.TargetCompID)
		if err := quickfix.SendToTarget(posRep, sID); err != nil {
			return err
		}
	}

	return nil
}

// FIXBalanceUpdateHandler is for balance updates
func (w *Websocket) FIXBalanceUpdateHandler(s *bitfinex.BalanceUpdate, sID quickfix.SessionID) error {
	info := bitfinex.BalanceInfo(*s)
	return w.FIXBalanceInfoHandler(&info, sID)
}

// FIXBalanceInfoHandler is for balance info
func (w *Websocket) FIXBalanceInfoHandler(s *bitfinex.BalanceInfo, sID quickfix.SessionID) error {
	wallet := bitfinex.WalletUpdate{
		Type:             "balance",
		Currency:         "all",
		Balance:          s.TotalAUM,
		BalanceAvailable: s.NetAUM,
	}
	return w.FIXWalletUpdateHandler(&wallet, sID)
}
