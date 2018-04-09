package peer

import (
	"fmt"
	"github.com/quickfixgo/enum"
	"go.uber.org/zap"
	"log"
	"sync"
)

type execution struct {
	BfxExecutionID string
	Px, Qty        float64
}

type CachedCancel struct {
	OriginalOrderID, Symbol, Account, ClOrdID string
	Side                                      enum.Side
}

// details BFX might not return back to us, which we need to populate in execution reports.
type CachedOrder struct {
	Symbol, Account  string
	ClOrdID, OrderID string
	Px, Qty          float64 // original px & qty
	Executions       []execution
	lock             sync.Mutex
	Side             enum.Side
	OrderType        enum.OrdType
}

func newOrder(clordid string, px, qty float64, symbol, account string, side enum.Side, ordType enum.OrdType) *CachedOrder {
	return &CachedOrder{
		ClOrdID:    clordid,
		Px:         px,
		Qty:        qty,
		Executions: make([]execution, 0),
		Symbol:     symbol,
		Account:    account,
		Side:       side,
		OrderType:  ordType,
	}
}

func newCancel(origclordid, symbol, account, clordid string) *CachedCancel {
	return &CachedCancel{
		OriginalOrderID: origclordid,
		Symbol:          symbol,
		Account:         account,
		ClOrdID:         clordid,
	}
}

func (o *CachedOrder) AvgFillPx() float64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.avgFillPx()
}

func (o *CachedOrder) avgFillPx() float64 {
	tot := 0.0
	n := len(o.Executions)
	for _, e := range o.Executions {
		tot = tot + e.Px
	}
	if n > 0 {
		return tot / float64(n)
	}
	return 0
}

func (o *CachedOrder) FilledQty() float64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.filledQty()
}

func (o *CachedOrder) filledQty() float64 {
	qty := 0.0
	for _, e := range o.Executions {
		qty = qty + e.Qty
	}
	return qty
}

// Stats provides clordid, qty, filled qty, avg px
func (o *CachedOrder) Stats() (string, float64, float64, float64) {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.ClOrdID, o.Qty, o.filledQty(), o.avgFillPx()
}

type cache struct {
	orders        map[string]*CachedOrder
	cancels       map[string]*CachedCancel
	mdReqIDs      map[string]string // FIX req ID -> Websocket req ID
	symbolToReqID map[string]string // symbol -> FIX req ID, for looking up FIX req IDs
	lock          sync.Mutex
	log           *zap.Logger
}

func newCache(log *zap.Logger) *cache {
	return &cache{
		orders:        make(map[string]*CachedOrder),
		cancels:       make(map[string]*CachedCancel),
		log:           log,
		mdReqIDs:      make(map[string]string),
		symbolToReqID: make(map[string]string),
	}
}

func (c *cache) MapSymbolToReqID(symbol, mdReqID string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.symbolToReqID[symbol] = mdReqID
}

func (c *cache) LookupMDReqID(symbol string) (string, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	id, ok := c.symbolToReqID[symbol]
	return id, ok
}

func (c *cache) MapMDReqID(fixReqID, apiReqID string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.mdReqIDs[fixReqID] = apiReqID
}

func (c *cache) LookupAPIReqID(fixReqID string) (string, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	id, ok := c.mdReqIDs[fixReqID]
	return id, ok
}

// add when receiving a NewOrderSingle over FIX
func (c *cache) AddOrder(clordid string, px, qty float64, symbol, account string, side enum.Side, ordType enum.OrdType) *CachedOrder {
	if qty < 0 {
		qty = -qty
	}
	c.lock.Lock()
	c.log.Info("added order to cache", zap.String("ClOrdID", clordid), zap.Float64("Px", px), zap.Float64("Qty", qty))
	order := newOrder(clordid, px, qty, symbol, account, side, ordType)
	c.orders[clordid] = order
	c.lock.Unlock()
	return order
}

func (c *cache) dump() {
	for clordid, order := range c.orders {
		log.Printf("%s:\t%s", clordid, order.OrderID)
	}
}

// update when receiving a on-req with a server-assigned order ID
func (c *cache) UpdateOrder(clordid, orderid string) (*CachedOrder, error) {
	c.log.Info("updated order cache", zap.String("ClOrdID", clordid), zap.String("OrderID", orderid))
	c.lock.Lock()
	defer c.lock.Unlock()
	if order, ok := c.orders[clordid]; ok {
		order.OrderID = orderid
		return order, nil
	}
	return nil, fmt.Errorf("could not find order to update with ClOrdID %s", clordid)
}

func (c *cache) AddCancel(origclordid, symbol, account, clordid string) *CachedCancel {
	c.lock.Lock()
	cancel := newCancel(origclordid, symbol, account, clordid)
	c.cancels[clordid] = cancel
	c.lock.Unlock()
	return cancel
}

func (c *cache) LookupCancel(clordid string) (*CachedCancel, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if cxl, ok := c.cancels[clordid]; ok {
		return cxl, nil
	}
	return nil, fmt.Errorf("could not find cancel with ClOrdID %s", clordid)
}

func (c *cache) LookupCancelByOrigClOrdID(origclordid string) (*CachedCancel, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, cxl := range c.cancels {
		if cxl.OriginalOrderID == origclordid {
			return cxl, nil
		}
	}
	return nil, fmt.Errorf("could not find cancel with OrigClOrdID %s", origclordid)
}

// UpdateExecutionFill receives an execution update with an ID, price, qty and returns the total filled qty & average fill price.
func (c *cache) AddExecution(orderid, execid string, px, qty float64) (float64, float64, error) {
	if qty < 0 {
		qty = -qty
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, o := range c.orders {
		if o.OrderID != orderid {
			continue
		}
		o.lock.Lock()
		defer o.lock.Unlock()
		c.log.Info("added execution to cache", zap.String("OrderID", orderid), zap.String("BfxExecutionID", execid), zap.Float64("Px", px), zap.Float64("Qty", qty))
		o.Executions = append(o.Executions, execution{
			Px:             px,
			Qty:            qty,
			BfxExecutionID: execid,
		})
		return o.filledQty(), o.avgFillPx(), nil
	}
	return 0, 0, fmt.Errorf("could not find OrderID %s in cache", orderid)
}

func (c *cache) LookupByClOrdID(clordid string) (*CachedOrder, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if order, ok := c.orders[clordid]; ok {
		return order, nil
	}
	return nil, fmt.Errorf("could not find an order with ClOrdID %s", clordid)
}

func (c *cache) LookupByOrderID(orderid string) (*CachedOrder, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, order := range c.orders {
		if order.OrderID == orderid {
			return order, nil
		}
	}
	return nil, fmt.Errorf("could not find OrderID %s", orderid)
}

func (c *cache) LookupClOrdID(orderid string) (string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for clordid, order := range c.orders {
		if order.OrderID == orderid {
			return clordid, nil
		}
	}
	return "", fmt.Errorf("could not find ClOrdID for OrderID %s", orderid)
}
