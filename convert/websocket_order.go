package convert

import (
	"errors"
	"strconv"
	"strings"

	"github.com/knarz/bitfinex-api-go"
	uuid "github.com/satori/go.uuid"

	"github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/quickfix/enum"
	"github.com/quickfixgo/quickfix/field"
	"github.com/shopspring/decimal"

	fix44er "github.com/quickfixgo/quickfix/fix44/executionreport"
	//fix44mdsfr "github.com/quickfixgo/quickfix/fix44/marketdatasnapshotfullrefresh"
	fix44nos "github.com/quickfixgo/quickfix/fix44/newordersingle"

	fix42er "github.com/quickfixgo/quickfix/fix42/executionreport"
	fix42nos "github.com/quickfixgo/quickfix/fix42/newordersingle"
)

func OrderFromTermData(td []interface{}) (*bitfinex.WebSocketV2Order, error) {
	if len(td) < 26 {
		return nil, errors.New("not an order status")
	}

	// XXX: API docs say ID, GID, CID, MTS_CREATE, MTS_UPDATE are int but API returns float

	os := &bitfinex.WebSocketV2Order{
		ID:            int64(Float64OrZero(td[0])),
		GID:           int64(Float64OrZero(td[1])),
		CID:           int64(Float64OrZero(td[2])),
		Symbol:        StringOrEmpty(td[3]),
		MTSCreate:     int64(Float64OrZero(td[4])),
		MTSUpdate:     int64(Float64OrZero(td[5])),
		Amount:        Float64OrZero(td[6]),
		AmountOrig:    Float64OrZero(td[7]),
		Type:          StringOrEmpty(td[8]),
		TypePrev:      StringOrEmpty(td[9]),
		Flags:         Int64OrZero(td[12]),
		OrderStatus:   StringOrEmpty(td[13]),
		Price:         Float64OrZero(td[16]),
		PriceAvg:      Float64OrZero(td[17]),
		PriceTrailing: Float64OrZero(td[18]),
		PriceAuxLimit: Float64OrZero(td[19]),
		Notify:        BoolOrFalse(td[23]),
		Hidden:        BoolOrFalse(td[24]),
		PlacedID:      Int64OrZero(td[25]),
	}

	return os, nil
}

func OrdStatusFromWebSocketV2Order(o *bitfinex.WebSocketV2Order) field.OrdStatusField {
	switch strings.ToUpper(o.OrderStatus) {
	default:
		return field.NewOrdStatus(enum.OrdStatus_NEW)
	case "CANCELED":
		return field.NewOrdStatus(enum.OrdStatus_CANCELED)
	case "EXECUTED":
		return field.NewOrdStatus(enum.OrdStatus_FILLED)
	case "PARTIALLY FILLED":
		return field.NewOrdStatus(enum.OrdStatus_PARTIALLY_FILLED)
	}
}

func SideFromWebSocketV2Order(o *bitfinex.WebSocketV2Order) field.SideField {
	switch {
	case o.Amount > 0.0:
		return field.NewSide(enum.Side_BUY)
	case o.Amount < 0.0:
		return field.NewSide(enum.Side_SELL)
	default:
		return field.NewSide(enum.Side_UNDISCLOSED)
	}
}

func LeavesQtyFromWebSocketV2Order(o *bitfinex.WebSocketV2Order) field.LeavesQtyField {
	c := abs(o.AmountOrig) - abs(o.Amount)
	d := decimal.NewFromFloat(c)

	return field.NewLeavesQty(d, 2)
}

func abs(f float64) float64 {
	if f < 0.0 {
		return -f
	}

	return f
}

func CumQtyFromWebSocketV2Order(o *bitfinex.WebSocketV2Order) field.CumQtyField {
	d := decimal.NewFromFloat(o.Amount)

	return field.NewCumQty(d, 2)
}

func AvgPxFromWebSocketV2Order(o *bitfinex.WebSocketV2Order) field.AvgPxField {
	d := decimal.NewFromFloat(o.PriceAvg)

	return field.NewAvgPx(d, 2)
}

func FIX44ExecutionReportFromWebsocketV2Order(o *bitfinex.WebSocketV2Order) fix44er.ExecutionReport {
	e := fix44er.New(
		field.NewOrderID(strconv.FormatInt(o.ID, 10)),
		field.NewExecID(uuid.NewV4().String()), // XXX: Can we just take a random ID here?
		field.NewExecType(enum.ExecType_ORDER_STATUS),
		OrdStatusFromWebSocketV2Order(o),
		SideFromWebSocketV2Order(o),
		LeavesQtyFromWebSocketV2Order(o),
		CumQtyFromWebSocketV2Order(o),
		AvgPxFromWebSocketV2Order(o),
	)

	e.SetSymbol(o.Symbol)
	e.SetClOrdID(strconv.FormatInt(o.CID, 10))

	return e
}

func FIX42ExecutionReportFromWebsocketV2Order(o *bitfinex.WebSocketV2Order) fix42er.ExecutionReport {
	e := fix42er.New(
		field.NewOrderID(strconv.FormatInt(o.ID, 10)),
		field.NewExecID(uuid.NewV4().String()), // XXX: Can we just take a random ID here?
		field.NewExecTransType(enum.ExecTransType_STATUS),
		field.NewExecType(enum.ExecType_ORDER_STATUS),

		OrdStatusFromWebSocketV2Order(o),
		field.NewSymbol(o.Symbol),
		SideFromWebSocketV2Order(o),
		LeavesQtyFromWebSocketV2Order(o),
		CumQtyFromWebSocketV2Order(o),
		AvgPxFromWebSocketV2Order(o),
	)

	e.SetClOrdID(strconv.FormatInt(o.CID, 10))

	return e
}

func FIX44ExecutionReportRejectUnknown(oid, cid string) fix44er.ExecutionReport {
	e := fix44er.New(
		field.NewOrderID(oid),
		field.NewExecID(uuid.NewV4().String()), // XXX: Can we just take a random ID here?
		field.NewExecType(enum.ExecType_ORDER_STATUS),
		field.NewOrdStatus(enum.OrdStatus_REJECTED),
		field.NewSide(enum.Side_UNDISCLOSED),
		field.NewLeavesQty(decimal.NewFromFloat(0.0), 2),
		field.NewCumQty(decimal.NewFromFloat(0.0), 2),
		field.NewAvgPx(decimal.NewFromFloat(0.0), 2),
	)

	e.SetClOrdID(cid)
	e.SetOrdRejReason(enum.OrdRejReason_UNKNOWN_ORDER)

	return e
}

func FIX42ExecutionReportRejectUnknown(oid, cid string) fix42er.ExecutionReport {
	e := fix42er.New(
		field.NewOrderID(oid),
		field.NewExecID(uuid.NewV4().String()), // XXX: Can we just take a random ID here?
		field.NewExecTransType(enum.ExecTransType_STATUS),
		field.NewExecType(enum.ExecType_ORDER_STATUS),

		field.NewOrdStatus(enum.OrdStatus_REJECTED),
		field.NewSymbol(""),
		field.NewSide(enum.Side_UNDISCLOSED),
		field.NewLeavesQty(decimal.NewFromFloat(0.0), 2),
		field.NewCumQty(decimal.NewFromFloat(0.0), 2),
		field.NewAvgPx(decimal.NewFromFloat(0.0), 2),
	)

	e.SetClOrdID(cid)
	e.SetOrdRejReason(enum.OrdRejReason_UNKNOWN_ORDER)

	return e
}

// OrderNewTypeFromFIX44NewOrderSingle takes a FIX44 NewOrderSingle and tries to extract enough information
// to figure out the appropriate type for the bitfinex order.
// XXX: Only works for EXCHANGE orders at the moment, i.e. automatically adds EXCHANGE prefix.
func OrderNewTypeFromFIX44NewOrderSingle(nos fix44nos.NewOrderSingle) string {
	ot, _ := nos.GetOrdType()
	tif, _ := nos.GetTimeInForce()
	ei, _ := nos.GetExecInst()

	pref := "EXCHANGE "

	switch {
	case ot == enum.OrdType_MARKET:
		return pref + "MARKET"
	case ot == enum.OrdType_LIMIT:
		return pref + "LIMIT"
	case ot == enum.OrdType_STOP:
		return pref + "STOP"
	case ot == enum.OrdType_STOP_LIMIT:
		return "STOP LIMIT"
	case tif == enum.TimeInForce_FILL_OR_KILL:
		return pref + "FOK"
	case ei == enum.ExecInst_ALL_OR_NONE && tif == enum.TimeInForce_IMMEDIATE_OR_CANCEL:
		return pref + "FOK"
	default:
		return ""
	}
}

// OrderNewTypeFromFIX42NewOrderSingle takes a FIX42 NewOrderSingle and tries to extract enough information
// to figure out the appropriate type for the bitfinex order.
// XXX: Only works for EXCHANGE orders at the moment, i.e. automatically adds EXCHANGE prefix.
func OrderNewTypeFromFIX42NewOrderSingle(nos fix42nos.NewOrderSingle) string {
	ot, _ := nos.GetOrdType()
	tif, _ := nos.GetTimeInForce()
	ei, _ := nos.GetExecInst()

	pref := "EXCHANGE "

	switch {
	case ot == enum.OrdType_MARKET:
		return pref + "MARKET"
	case ot == enum.OrdType_LIMIT:
		return pref + "LIMIT"
	case ot == enum.OrdType_STOP:
		return pref + "STOP"
	case ot == enum.OrdType_STOP_LIMIT:
		return "STOP LIMIT"
	case tif == enum.TimeInForce_FILL_OR_KILL:
		return pref + "FOK"
	case ei == enum.ExecInst_ALL_OR_NONE && tif == enum.TimeInForce_IMMEDIATE_OR_CANCEL:
		return pref + "FOK"
	default:
		return ""
	}
}

// WebSocketV2OrderNewFromFIX44NewOrderSingle converts a NewOrderSingle into a new order for the
// bitfinex websocket API, as best as it can.
func WebSocketV2OrderNewFromFIX44NewOrderSingle(nos fix44nos.NewOrderSingle) (*bitfinex.WebSocketV2OrderNew, quickfix.MessageRejectError) {
	on := &bitfinex.WebSocketV2OrderNew{}

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

	on.Type = OrderNewTypeFromFIX44NewOrderSingle(nos)

	s, err := nos.GetSymbol()
	if err != nil {
		return nil, err
	}
	on.Symbol = s

	qd, err := nos.GetOrderQty()
	if err != nil {
		return nil, err
	}
	q, _ := qd.Float64()
	on.Amount = q

	pd, err := nos.GetPrice()
	if err != nil {
		return nil, err
	}
	p, _ := pd.Float64()

	side, err := nos.GetSide()
	if err != nil {
		return nil, err
	}

	if side == enum.Side_SELL {
		on.Price = -p
	} else if side == enum.Side_BUY {
		on.Price = p
	}

	return on, nil
}

// WebSocketV2OrderNewFromFIX42NewOrderSingle converts a NewOrderSingle into a new order for the
// bitfinex websocket API, as best as it can.
func WebSocketV2OrderNewFromFIX42NewOrderSingle(nos fix42nos.NewOrderSingle) (*bitfinex.WebSocketV2OrderNew, quickfix.MessageRejectError) {
	on := &bitfinex.WebSocketV2OrderNew{}

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

	on.Type = OrderNewTypeFromFIX42NewOrderSingle(nos)

	s, err := nos.GetSymbol()
	if err != nil {
		return nil, err
	}
	on.Symbol = s

	qd, err := nos.GetOrderQty()
	if err != nil {
		return nil, err
	}
	q, _ := qd.Float64()
	on.Amount = q

	pd, err := nos.GetPrice()
	if err != nil {
		return nil, err
	}
	p, _ := pd.Float64()

	side, err := nos.GetSide()
	if err != nil {
		return nil, err
	}

	if side == enum.Side_SELL {
		on.Price = -p
	} else if side == enum.Side_BUY {
		on.Price = p
	}

	return on, nil
}
