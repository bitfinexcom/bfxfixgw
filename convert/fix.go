package convert

import (
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/shopspring/decimal"
)

// Generic FIX types.

func OrdStatusToFIX(status bitfinex.OrderStatus) enum.OrdStatus {
	switch status {
	default:
		return enum.OrdStatus_NEW
	case bitfinex.OrderStatusCanceled:
		return enum.OrdStatus_CANCELED
	case bitfinex.OrderStatusPartiallyFilled:
		return enum.OrdStatus_PARTIALLY_FILLED
	case bitfinex.OrderStatusExecuted:
		return enum.OrdStatus_FILLED
	}
}

// follows FIX 4.1+ rules on merging ExecTransType + ExecType fields into new ExecType enums.
func ExecTypeToFIX(status bitfinex.OrderStatus) field.ExecTypeField {
	switch status {
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

func SideToFIX(amount float64) field.SideField {
	switch {
	case amount > 0.0:
		return field.NewSide(enum.Side_BUY)
	case amount < 0.0:
		return field.NewSide(enum.Side_SELL)
	default:
		return field.NewSide(enum.Side_UNDISCLOSED)
	}
}

// qty
func LeavesQtyToFIX(amount float64) field.LeavesQtyField {
	d := decimal.NewFromFloat(amount)
	return field.NewLeavesQty(d, 4)
}

// qty
func LastSharesToFIX(qty float64) field.LastSharesField {
	d := decimal.NewFromFloat(qty)
	return field.NewLastShares(d, 4)
}

func CumQtyToFIX(cumQty float64) field.CumQtyField {
	return field.NewCumQty(decimal.NewFromFloat(cumQty), 2)
}

func AvgPxToFIX(priceAvg float64) field.AvgPxField {
	d := decimal.NewFromFloat(priceAvg)
	return field.NewAvgPx(d, 2)
}
