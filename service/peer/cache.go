package peer

import (
	"fmt"
	"go.uber.org/zap"
	"sync"
)

type execution struct {
	BfxExecutionID string
	Px, Qty        float64
}

type Order struct {
	ClOrdID, OrderID string
	Px, Qty          float64 // original px & qty
	Executions       []execution
	lock             sync.Mutex
}

func newOrder(orderid, clordid string, px, qty float64) *Order {
	return &Order{
		ClOrdID:    clordid,
		OrderID:    orderid,
		Px:         px,
		Qty:        qty,
		Executions: make([]execution, 0),
	}
}

func (o *Order) AvgFillPx() float64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.avgFillPx()
}

func (o *Order) avgFillPx() float64 {
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

func (o *Order) FilledQty() float64 {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.filledQty()
}

func (o *Order) filledQty() float64 {
	qty := 0.0
	for _, e := range o.Executions {
		qty = qty + e.Qty
	}
	return qty
}

// Stats provides clordid, qty, filled qty, avg px
func (o *Order) Stats() (string, float64, float64, float64) {
	o.lock.Lock()
	defer o.lock.Unlock()
	return o.ClOrdID, o.Qty, o.filledQty(), o.avgFillPx()
}

type cache struct {
	orders map[string]*Order
	lock   sync.Mutex
	log    *zap.Logger
}

func newCache(log *zap.Logger) *cache {
	return &cache{
		orders: make(map[string]*Order),
		log:    log,
	}
}

func (c *cache) AddOrder(orderid, clordid string, px, qty float64) *Order {
	if qty < 0 {
		qty = -qty
	}
	c.lock.Lock()
	c.log.Info("added order to cache", zap.String("OrderID", orderid), zap.String("ClOrdID", clordid), zap.Float64("Px", px), zap.Float64("Qty", qty))
	order := newOrder(orderid, clordid, px, qty)
	c.orders[orderid] = order
	c.lock.Unlock()
	return order
}

// UpdateExecutionFill receives an execution update with an ID, price, qty and returns the total filled qty & average fill price.
func (c *cache) AddExecution(orderid, execid string, px, qty float64) (float64, float64, error) {
	if qty < 0 {
		qty = -qty
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	if o, ok := c.orders[orderid]; ok {
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

func (c *cache) Lookup(orderid string) (*Order, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if o, ok := c.orders[orderid]; ok {
		return o, nil
	}
	return nil, fmt.Errorf("could not find OrderID %s", orderid)
}

func (c *cache) LookupClOrdID(orderid string) (string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if o, ok := c.orders[orderid]; ok {
		return o.ClOrdID, nil
	}
	return "", fmt.Errorf("could not find ClOrdID for OrderID %s", orderid)
}
