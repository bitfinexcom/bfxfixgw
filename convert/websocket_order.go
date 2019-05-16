package convert

import (
	"github.com/quickfixgo/field"
	"github.com/shopspring/decimal"
	"strconv"
	"strings"

	"github.com/bitfinexcom/bitfinex-api-go/v2"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/tag"

	"github.com/bitfinexcom/bfxfixgw/service/symbol"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
)

// TimeInForceFormat is the string format required for a dynamic expiration date
const TimeInForceFormat = "2006-01-02 15:04:05"

// OrderNewTypeFromFIX42 takes a FIX42 message and tries to extract enough information
// to figure out the appropriate type for the bitfinex order.
func OrderNewTypeFromFIX42(msg quickfix.FieldMap) (ordType string, err quickfix.MessageRejectError) {
	ot := &field.OrdTypeField{}
	_ = msg.Get(ot)
	tif := &field.TimeInForceField{}
	_ = msg.Get(tif)
	ei := &field.ExecInstField{}
	_ = msg.Get(ei)
	cm := &field.CashMarginField{}
	_ = msg.Get(cm)

	if ei.Value() == enum.ExecInst_ALL_OR_NONE {
		err = quickfix.ValueIsIncorrect(ei.Tag())
		return
	}

	// map AON & IOC => FOK
	if tif.Value() == enum.TimeInForce_FILL_OR_KILL || tif.Value() == enum.TimeInForce_IMMEDIATE_OR_CANCEL {
		ordType = bitfinex.OrderTypeExchangeFOK
	} else {
		switch ot.Value() {
		case enum.OrdType_MARKET:
			ordType = bitfinex.OrderTypeExchangeMarket
		case enum.OrdType_LIMIT:
			ordType = bitfinex.OrderTypeExchangeLimit
		case enum.OrdType_STOP:
			ordType = bitfinex.OrderTypeExchangeStop
			if msg.Has(ei.Tag()) && strings.Contains(string(ei.Value()), string(enum.ExecInst_PRIMARY_PEG)) {
				ordType = bitfinex.OrderTypeExchangeTrailingStop
			}
		case enum.OrdType_STOP_LIMIT:
			ordType = bitfinex.OrderTypeExchangeStopLimit
		}
	}

	// if cash margin flag present, swizzle order type prefix
	if msg.Has(tag.CashMargin) && cm.Value() == enum.CashMargin_MARGIN_OPEN {
		ordType = strings.Replace(ordType, "EXCHANGE", "MARGIN", 1)
	}
	return
}

// GetTimeInForceFromFIX extracts a FIX message map's time in force information
func GetTimeInForceFromFIX(msg quickfix.FieldMap) (tif enum.TimeInForce, tifmts string, err quickfix.MessageRejectError) {
	tif = enum.TimeInForce_GOOD_TILL_CANCEL // default TIF
	timeInForce := &field.TimeInForceField{}
	if msg.Has(timeInForce.Tag()) {
		if err = msg.Get(timeInForce); err != nil {
			return
		} else if timeInForce.Value() == enum.TimeInForce_GOOD_TILL_DATE {
			expirationDate := &field.ExpireTimeField{}
			if err = msg.Get(expirationDate); err != nil {
				return
			}
			tifmts = expirationDate.Value().Format(TimeInForceFormat)
		}
	}
	return
}

// GetAmountFromQtyAndSide converts a side and quantity to a Bitfinex amount
func GetAmountFromQtyAndSide(side enum.Side, qty decimal.Decimal) (amount float64) {
	amount, _ = qty.Float64()
	if side == enum.Side_SELL {
		amount = -amount
	}
	return
}

// GetFlagsFromFIX extracts order flags from FIX message
func GetFlagsFromFIX(msg quickfix.FieldMap) (hidden bool, postOnly bool, oco bool) {
	// hidden
	displayMethod, err := msg.GetString(tag.DisplayMethod)
	if err == nil && enum.DisplayMethod(displayMethod) == enum.DisplayMethod_UNDISCLOSED {
		hidden = true
	}

	// post only
	execInst := &field.ExecInstField{}
	err = msg.Get(execInst)
	if err == nil && strings.Contains(string(execInst.Value()), string(enum.ExecInst_PARTICIPANT_DONT_INITIATE)) {
		postOnly = true
	}

	// order cancels order
	if msg.Has(tag.ContingencyType) {
		ctf := &field.ContingencyTypeField{}
		if err = msg.Get(ctf); err == nil {
			oco = ctf.Value() == enum.ContingencyType_ONE_CANCELS_THE_OTHER
		}
	}
	return
}

// GetAmountFromQtyAndSide converts an order with type and prices to Bitfinex price and aux price
func GetPricesFromOrdType(msg quickfix.FieldMap) (t enum.OrdType, px float64, auxpx float64, trailpx float64, ocopx float64, err quickfix.MessageRejectError) {
	tf := &field.OrdTypeField{}
	if err = msg.Get(tf); err != nil {
		return
	}
	t = tf.Value()

	switch t {
	case enum.OrdType_LIMIT:
		pd := &field.PriceField{}
		if err = msg.Get(pd); err != nil {
			return
		}
		px, _ = pd.Float64()
		if msg.Has(tag.ContingencyType) {
			ctf := &field.ContingencyTypeField{}
			if err = msg.Get(ctf); err != nil {
				return
			} else if ctf.Value() == enum.ContingencyType_ONE_CANCELS_THE_OTHER {
				pd := &field.StopPxField{}
				if err = msg.Get(pd); err != nil {
					return
				}
				ocopx, _ = pd.Float64()
			}
		}
	case enum.OrdType_STOP:
		pd := &field.StopPxField{}
		if err = msg.Get(pd); err != nil {
			return
		}
		px, _ = pd.Float64()
		// check trailing stop
		execInst := &field.ExecInstField{}
		if msg.Has(execInst.Tag()) {
			err = msg.Get(execInst)
			if err == nil && strings.Contains(string(execInst.Value()), string(enum.ExecInst_PRIMARY_PEG)) {
				trail := &field.PegDifferenceField{}
				if err = msg.Get(trail); err != nil {
					return // trailing stop needs a peg
				}
				trailpx, _ = trail.Float64()
			}
		}
	case enum.OrdType_STOP_LIMIT:
		lm := &field.PriceField{}
		if err = msg.Get(lm); err != nil {
			return
		}
		px, _ = lm.Float64()
		pd := &field.StopPxField{}
		if err = msg.Get(pd); err != nil {
			return
		}
		auxpx, _ = pd.Float64()
	}
	return
}

// OrderNewFromFIX42NewOrderSingle converts a NewOrderSingle into a new order for the
// bitfinex websocket API, as best as it can.
func OrderNewFromFIX42NewOrderSingle(nos fix42nos.NewOrderSingle, symbology symbol.Symbology, counterparty string) (*bitfinex.OrderNewRequest, quickfix.MessageRejectError) {
	on := &bitfinex.OrderNewRequest{}

	on.GID = 0
	cidstr, err := nos.GetClOrdID()
	if err != nil {
		return nil, err
	}
	cid, perr := strconv.ParseInt(cidstr, 10, 64)
	if perr != nil {
		cid = 0
	}
	on.CID = cid

	on.Type, err = OrderNewTypeFromFIX42(nos.FieldMap)
	if err != nil {
		return nil, err
	}

	s, err := nos.GetSymbol()
	if err != nil {
		return nil, err
	}
	translated, err2 := symbology.ToBitfinex(s, counterparty)
	var sym string
	if err2 == nil {
		sym = translated
	} else {
		sym = s
	}
	on.Symbol = sym

	_, on.Price, on.PriceAuxLimit, on.PriceTrailing, on.PriceOcoStop, err = GetPricesFromOrdType(nos.FieldMap)
	if err != nil {
		return nil, err
	}

	side, err := nos.GetSide()
	if err != nil {
		return nil, err
	}

	qd, err := nos.GetOrderQty()
	if err != nil {
		return nil, err
	}
	on.Amount = GetAmountFromQtyAndSide(side, qd)

	on.Hidden, on.PostOnly, on.OcoOrder = GetFlagsFromFIX(nos.FieldMap)
	return on, nil
}
