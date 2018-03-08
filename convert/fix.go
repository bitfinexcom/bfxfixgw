package convert

import (
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/shopspring/decimal"
)

// Generic FIX types.

func OrdStatusFromOrder(o *bitfinex.Order) field.OrdStatusField {
	switch o.Status {
	default:
		return field.NewOrdStatus(enum.OrdStatus_NEW)
	case bitfinex.OrderStatusCanceled:
		return field.NewOrdStatus(enum.OrdStatus_CANCELED)
	case bitfinex.OrderStatusPartiallyFilled:
		return field.NewOrdStatus(enum.OrdStatus_PARTIALLY_FILLED)
	case bitfinex.OrderStatusExecuted:
		return field.NewOrdStatus(enum.OrdStatus_FILLED)
	}
}

// follows FIX 4.1+ rules on merging ExecTransType + ExecType fields into new ExecType enums.
func ExecTypeFromOrder(o *bitfinex.Order) field.ExecTypeField {
	switch o.Status {
	default:
		return field.NewExecType(enum.ExecType_ORDER_STATUS)
	case bitfinex.OrderStatusActive:
		return field.NewExecType(enum.ExecType_NEW)
	case bitfinex.OrderStatusCanceled:
		return field.NewExecType(enum.ExecType_TRADE_CANCEL)
	case bitfinex.OrderStatusPartiallyFilled:
		return field.NewExecType(enum.ExecType_TRADE)
	case bitfinex.OrderStatusExecuted:
		return field.NewExecType(enum.ExecType_TRADE)
	}
}

func SideFromOrder(o *bitfinex.Order) field.SideField {
	switch {
	case o.Amount > 0.0:
		return field.NewSide(enum.Side_BUY)
	case o.Amount < 0.0:
		return field.NewSide(enum.Side_SELL)
	default:
		return field.NewSide(enum.Side_UNDISCLOSED)
	}
}

func LeavesQtyFromOrder(o *bitfinex.Order) field.LeavesQtyField {
	d := decimal.NewFromFloat(o.Amount)
	return field.NewLeavesQty(d, 2)
}

func CumQtyFromOrder(o *bitfinex.Order) field.CumQtyField {
	a := decimal.NewFromFloat(o.AmountOrig)
	b := decimal.NewFromFloat(o.Amount)
	return field.NewCumQty(a.Sub(b.Abs()), 2)
}

func AvgPxFromOrder(o *bitfinex.Order) field.AvgPxField {
	d := decimal.NewFromFloat(o.PriceAvg)
	return field.NewAvgPx(d, 2)
}
