package main

import (
	"time"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	"github.com/shopspring/decimal"
)

//TestNewOrderSingleBuyLimitFill assures the gateway service will publish an OrderNew websocket message when receiving a FIX42 NewOrderSingle
func (s *gatewaySuite) TestNewOrderSingleBuyLimitFill() {
	// assert FIX MD logon
	fix, err := s.fixMd.WaitForMessage(s.MarketDataSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	s.Require().Nil(err)
	// assert FIX order logon
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	s.Require().Nil(err)

	// assume both ws clients connected in setup()
	s.srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := s.srvWs.WaitForMessage(MarketDataClient, 0)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}`, msg)

	// assert order ws auth request
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 0)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}`, msg)

	// broadcast auth ack to both clients
	s.srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(12000.0), 1))
	session := s.fixOrd.LastSession()
	err = session.Send(nos)
	s.Require().Nil(err)

	// assert OrderNew
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 1)
	s.Require().Nil(err)
	s.Require().EqualValues(`[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE LIMIT","symbol":"BTCUSD","amount":"1","price":"12000"}]`, msg)

	// service publish pending new ack
	s.srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"EXCHANGE LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// service publish new ack, assert NEW
	s.srvWs.Send(OrdersClient, `[0,"on",[1234567,0,555,"tBTCUSD",1521153050972,1521153051035,1,1,"EXCHANGE LIMIT",null,null,null,0,"ACTIVE",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00")
	s.Require().Nil(err)

	// trade execution
	s.srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.21679716,12000,"EXCHANGE LIMIT",12000,1,-0.39712904,"USD"]]`)

	// assert FIX execution report PARTIAL FILL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 3)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.2168", "39=1", "54=1", "55=tBTCUSD", "150=1", "151=0.7832", "6=12000", "14=0.2168")
	s.Require().Nil(err)

	// trade execution
	s.srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.78320284,12000,"EXCHANGE LIMIT",12000,1,-0.39712904,"USD"]]`)

	// assert FIX execution report FULL FILL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 4)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.7832", "39=2", "54=1", "55=tBTCUSD", "150=2", "151=0.0000", "6=12000", "14=1.000")
	s.Require().Nil(err)
}

func (s *gatewaySuite) TestNewOrderSingleSellMarketFill() {
	// assert FIX MD logon
	fix, err := s.fixMd.WaitForMessage(s.MarketDataSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	s.Require().Nil(err)
	// assert FIX order logon
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	s.Require().Nil(err)

	// assume both ws clients connected in setup()
	s.srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := s.srvWs.WaitForMessage(MarketDataClient, 0)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}`, msg)

	// assert order ws auth request
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 0)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}`, msg)

	// broadcast auth ack to both clients
	s.srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("tBTCUSD"),
		field.NewSide(enum.Side_SELL),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_MARKET))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	session := s.fixOrd.LastSession()
	err = session.Send(nos)
	s.Require().Nil(err)

	// assert OrderNew
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 1)
	s.Require().Nil(err)
	s.Require().EqualValues(`[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE MARKET","symbol":"tBTCUSD","amount":"-1","price":"0"}]`, msg)

	// service publish pending new ack
	s.srvWs.Send(OrdersClient, `[0,"n",[1521235125435,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,-1,-1,"EXCHANGE MARKET",null,null,null,null,null,null,null,1637.4,null,null,null,null,null,null,0,null,null,null,null,null,null,null,null],null,"SUCCESS","Submitting exchange market sell order for 1.0 BTC."]]`)

	// assert FIX execution report NEW
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=1", "20=3", "32=0.000", "39=0", "54=2", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00")
	s.Require().Nil(err)

	// trade execution
	s.srvWs.Send(OrdersClient, `[0,"tu",[701,"tBTCUSD",1521235125445,1234567,-0.15299251,12000,"EXCHANGE MARKET",12000,-1,-0.50095867,"USD"]]`)

	// assert FIX execution report PARTIAL FILL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 3)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=1", "20=3", "32=0.1530", "39=1", "54=2", "55=tBTCUSD", "150=1", "151=0.8470", "6=12000", "14=0.1530")
	s.Require().Nil(err)

	// trade execution
	s.srvWs.Send(OrdersClient, `[0,"tu",[701,"tBTCUSD",1521235125445,1234567,-0.21845811,12000,"EXCHANGE MARKET",12000,-1,-0.50095867,"USD"]]`)

	// assert FIX execution report PARTIAL FILL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 4)
	s.Require().Nil(err)
	// note: tag 32 rounding
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=1", "20=3", "32=0.2185", "39=1", "54=2", "55=tBTCUSD", "150=1", "151=0.6285", "6=12000", "14=0.3715")
	s.Require().Nil(err)

	// trade execution
	s.srvWs.Send(OrdersClient, `[0,"tu",[701,"tBTCUSD",1521235125445,1234567,-0.62854938,12000,"EXCHANGE MARKET",12000,-1,-0.50095867,"USD"]]`)

	// assert FIX execution report FULL FILL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 5)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=1", "20=3", "32=0.6285", "39=2", "54=2", "55=tBTCUSD", "150=2", "151=0.0000", "6=12000", "14=1.000")
	s.Require().Nil(err)
}

func (s *gatewaySuite) TestNewOrderSingleRejectBadPrice() {
	// assert FIX MD logon
	fix, err := s.fixMd.WaitForMessage(s.MarketDataSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	s.Require().Nil(err)
	// assert FIX order logon
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	s.Require().Nil(err)

	// assume both ws clients connected in setup()
	s.srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := s.srvWs.WaitForMessage(MarketDataClient, 0)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}`, msg)

	// assert order ws auth request
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 0)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}`, msg)

	// broadcast auth ack to both clients
	s.srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("tBTCUSD"),
		field.NewSide(enum.Side_SELL),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_LIMIT))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	nos.Set(field.NewPrice(decimal.NewFromFloat(-12483), 4)) // bad price
	session := s.fixOrd.LastSession()
	err = session.Send(nos)
	s.Require().Nil(err)

	// assert OrderNew
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 1)
	s.Require().Nil(err)
	s.Require().EqualValues(`[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE LIMIT","symbol":"tBTCUSD","amount":"-1","price":"-12483"}]`, msg)

	// service publish pending new ack
	s.srvWs.Send(OrdersClient, `[0,"n",[1521237838790,"on-req",null,null,[null,null,555,"tBTCUSD",null,null,-1,null,"EXCHANGE LIMIT",null,null,null,null,null,null,null,-12483,null,null,null,null,null,null,0,null,null,null,null,null,null,null,null],null,"ERROR","Invalid price."]]`)

	// assert FIX execution report reject
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=2", "20=3", "32=0.000", "39=8", "54=2", "55=tBTCUSD", "150=8", "151=1.00", "6=0.00", "14=0.00")
	s.Require().Nil(err)
}

func (s *gatewaySuite) TestNewOrderSingleRejectBadSymbol() {
	// assert FIX MD logon
	fix, err := s.fixMd.WaitForMessage(s.MarketDataSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	s.Require().Nil(err)
	// assert FIX order logon
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	s.Require().Nil(err)

	// assume both ws clients connected in setup()
	s.srvWs.Broadcast(`{"event":"info","version":2}`)

	// assert MD ws auth request
	msg, err := s.srvWs.WaitForMessage(MarketDataClient, 0)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}`, msg)

	// assert order ws auth request
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 0)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce1","event":"auth","apiKey":"apiKey1","authSig":"2744ec1afc974eadbda7e09efa03da80578628ba90e2aa5fcba8c2c61014b811f3a8be5a041c3ee35c464a59856b3869","authPayload":"AUTHnonce1","authNonce":"nonce1"}`, msg)

	// broadcast auth ack to both clients
	s.srvWs.Broadcast(`{"event":"auth","status":"OK","chanId":0,"userId":1,"subId":"nonce1","auth_id":"valid-auth-guid","caps":{"orders":{"read":1,"write":0},"account":{"read":1,"write":0},"funding":{"read":1,"write":0},"history":{"read":1,"write":0},"wallets":{"read":1,"write":0},"withdraw":{"read":0,"write":0},"positions":{"read":1,"write":0}}}`)

	// send NOS, ClOrdID MUST be an integer
	nos := fix42nos.New(field.NewClOrdID("555"),
		field.NewHandlInst(enum.HandlInst_MANUAL_ORDER_BEST_EXECUTION),
		field.NewSymbol("tBTCUSD"),
		field.NewSide(enum.Side_SELL),
		field.NewTransactTime(time.Now()),
		field.NewOrdType(enum.OrdType_MARKET))
	nos.Set(field.NewOrderQty(decimal.NewFromFloat(1.0), 1))
	session := s.fixOrd.LastSession()
	err = session.Send(nos)
	s.Require().Nil(err)

	// assert OrderNew
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 1)
	s.Require().Nil(err)
	s.Require().EqualValues(`[0,"on",null,{"gid":0,"cid":555,"type":"EXCHANGE MARKET","symbol":"tBTCUSD","amount":"-1","price":"0"}]`, msg)

	// service publish pending new ack
	s.srvWs.Send(OrdersClient, `[0,"n",[1521237838790,"on-req",null,null,[null,null,555,"tBTCUSD",null,null,-1,null,"EXCHANGE MARKET",null,null,null,null,null,null,null,-12483,null,null,null,null,null,null,0,null,null,null,null,null,null,null,null],null,"ERROR","Invalid price."]]`)

	// assert FIX execution report reject
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "40=1", "20=3", "32=0.000", "39=8", "54=2", "55=tBTCUSD", "150=8", "151=1.00", "6=0.00", "14=0.00")
	s.Require().Nil(err)
}
