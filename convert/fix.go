package convert

import (
	"github.com/bitfinexcom/bitfinex-api-go/v2"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/shopspring/decimal"
	"strings"
)

// Generic FIX types.

func OrdStatusToFIX(status bitfinex.OrderStatus) enum.OrdStatus {
	// if the status is a composite (e.g. EXECUTED @ X: was PARTIALLY FILLED @ Y)
	if strings.HasPrefix(string(status), string(bitfinex.OrderStatusExecuted)) {
		return enum.OrdStatus_FILLED
	}
	if strings.HasPrefix(string(status), string(bitfinex.OrderStatusPartiallyFilled)) {
		return enum.OrdStatus_PARTIALLY_FILLED
	}
	if strings.HasPrefix(string(status), string(bitfinex.OrderStatusCanceled)) {
		return enum.OrdStatus_CANCELED
	}
	return enum.OrdStatus_NEW
}

// follows FIX 4.1+ rules on merging ExecTransType + ExecType fields into new ExecType enums.
func ExecTypeToFIX(status bitfinex.OrderStatus) enum.ExecType {
	if strings.HasPrefix(string(status), string(bitfinex.OrderStatusActive)) {
		return enum.ExecType_NEW
	}
	if strings.HasPrefix(string(status), string(bitfinex.OrderStatusCanceled)) {
		return enum.ExecType_TRADE_CANCEL
	}
	if strings.HasPrefix(string(status), string(bitfinex.OrderStatusPartiallyFilled)) {
		return enum.ExecType_TRADE
	}
	if strings.HasPrefix(string(status), string(bitfinex.OrderStatusExecuted)) {
		return enum.ExecType_TRADE
	}
	return enum.ExecType_ORDER_STATUS
}

func SideToFIX(amount float64) enum.Side {
	switch {
	case amount > 0.0:
		return enum.Side_BUY
	case amount < 0.0:
		return enum.Side_SELL
	default:
		return enum.Side_UNDISCLOSED
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

func OrdTypeToFIX(ordtype string) enum.OrdType {
	switch ordtype {
	case "EXCHANGE LIMIT":
		fallthrough
	case "LIMIT":
		return enum.OrdType_LIMIT
	case "EXCHANGE MARKET":
	case "MARKET":
		return enum.OrdType_MARKET
	}
	return enum.OrdType_MARKET
}
