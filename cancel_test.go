package main

import (
	"fmt"
	"time"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	fix42nos "github.com/quickfixgo/fix42/newordersingle"
	fix42cxl "github.com/quickfixgo/fix42/ordercancelrequest"
	"github.com/shopspring/decimal"
)

func (s *gatewaySuite) TestOrderCancelSimple() {
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

	// service publish new ack
	s.srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"EXCHANGE LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// service publish new working
	s.srvWs.Send(OrdersClient, `[0,"on",[1234567,0,555,"tBTCUSD",1521153050972,1521153051035,1,1,"EXCHANGE LIMIT",null,null,null,0,"ACTIVE",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)

	// assert FIX execution report NEW
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00")
	s.Require().Nil(err)

	// attempt to cancel order
	cxl := fix42cxl.New(field.NewOrigClOrdID("555"),
		field.NewClOrdID("556"),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()))
	err = session.Send(cxl)
	s.Require().Nil(err)

	// assert cancel req
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 2)
	s.Require().Nil(err)
	today := time.Now().Format("2006-01-02")
	f := `[0,"oc",null,{"cid":555,"cid_date":"%s"}]`
	exp := fmt.Sprintf(f, today)
	s.Require().EqualValues(exp, msg)

	// publish cancel ack
	s.srvWs.Send(OrdersClient, `[0,"n",[1521153051035,"oc-req",null,null,[null,null,555,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,0,null,null,null,null,null,null,null,null],null,"SUCCESS","Submitted for cancellation; waiting for confirmation (ID: 1234567)."]]`)
	// assert FIX PENDING CANCEL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 3)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "14=0.0000", "20=3", "14=0.0000", "37=1234567", "39=6", "54=1", "151=1.0000")
	s.Require().Nil(err)

	// publish cancel success
	s.srvWs.Send(OrdersClient, `[0,"oc",[1234567,0,555,"tBTCUSD",1521062529896,1521062593974,1,1,"EXCHANGE LIMIT",null,null,null,0,"CANCELED",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)
	// assert FIX CANCEL ack
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 4)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "14=0.0000", "20=3", "32=0.0000", "37=1234567", "39=4", "54=1", "55=tBTCUSD", "150=4")
	s.Require().Nil(err)
}

func (s *gatewaySuite) TestOrderCancelInFlightFillOK() {
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

	// service publish new ack
	s.srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"EXCHANGE LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// service publish new working
	s.srvWs.Send(OrdersClient, `[0,"on",[1234567,0,555,"tBTCUSD",1521153050972,1521153051035,1,1,"EXCHANGE LIMIT",null,null,null,0,"ACTIVE",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)

	// assert FIX execution report NEW
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00")
	s.Require().Nil(err)

	// trade execution
	s.srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.21679716,12000,"MARKET",12000,-1,-0.39712904,"USD"]]`)

	// assert FIX execution report PARTIAL FILL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 3)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.2168", "39=1", "54=1", "55=tBTCUSD", "150=1", "151=0.7832", "6=12000.00", "14=0.2168")
	s.Require().Nil(err)

	// attempt to cancel order
	cxl := fix42cxl.New(field.NewOrigClOrdID("555"),
		field.NewClOrdID("556"),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()))
	err = session.Send(cxl)
	s.Require().Nil(err)

	// assert cancel req
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 2)
	s.Require().Nil(err)
	today := time.Now().Format("2006-01-02")
	f := `[0,"oc",null,{"cid":555,"cid_date":"%s"}]`
	exp := fmt.Sprintf(f, today)
	s.Require().EqualValues(exp, msg)

	// publish cancel ack
	s.srvWs.Send(OrdersClient, `[0,"n",[1521153051035,"oc-req",null,null,[null,null,555,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,0,null,null,null,null,null,null,null,null],null,"SUCCESS","Submitted for cancellation; waiting for confirmation (ID: 1234567)."]]`)
	// assert FIX PENDING CANCEL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 4)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "14=0.2168", "37=1234567", "39=6", "54=1", "150=6", "151=0.7832")
	s.Require().Nil(err)

	// publish cancel success
	s.srvWs.Send(OrdersClient, `[0,"oc",[1234567,0,555,"tBTCUSD",1521062529896,1521062593974,1,1,"EXCHANGE LIMIT",null,null,null,0,"CANCELED",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)
	// assert FIX CANCEL ack
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 5)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "14=0.2168", "20=3", "37=1234567", "39=4", "54=1", "55=tBTCUSD", "150=4", "151=0.0000")
	s.Require().Nil(err)
}

func (s *gatewaySuite) TestOrderCancelUnknownOrder() {
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

	// attempt to cancel order
	cxl := fix42cxl.New(field.NewOrigClOrdID("555"),
		field.NewClOrdID("556"),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()))
	session := s.fixOrd.LastSession()
	err = session.Send(cxl)
	s.Require().Nil(err)

	// assert cancel req
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 1)
	s.Require().Nil(err)
	today := time.Now().Format("2006-01-02")
	f := `[0,"oc",null,{"cid":555,"cid_date":"%s"}]`
	exp := fmt.Sprintf(f, today)
	s.Require().EqualValues(exp, msg)

	// publish cancel error
	s.srvWs.Send(OrdersClient, `[0,"n",[1521231457686,"oc-req",null,null,[null,null,555,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,0,null,null,null,null,null,null,null,null],null,"ERROR","Order not found."]]`)
	// assert FIX cancel reject
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=9", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "11=555", "41=555", "39=8", "434=1", "102=1")
	s.Require().Nil(err)
}

func (s *gatewaySuite) TestOrderCancelInFlightFillReject() {
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

	// service publish new ack
	s.srvWs.Send(OrdersClient, `[0,"n",[null,"on-req",null,null,[1234567,null,555,"tBTCUSD",null,null,1,1,"EXCHANGE LIMIT",null,null,null,null,null,null,null,12000,null,null,null,null,null,null,0,null,null],null,"SUCCESS","Submitting limit buy order for 1.0 BTC."]]`)

	// service publish new working
	s.srvWs.Send(OrdersClient, `[0,"on",[1234567,0,555,"tBTCUSD",1521153050972,1521153051035,1,1,"EXCHANGE LIMIT",null,null,null,0,"ACTIVE",null,null,12000,0,null,null,null,null,null,0,0,0,null,null,"API>BFX",null,null,null]]`)

	// assert FIX execution report NEW
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.000", "39=0", "54=1", "55=tBTCUSD", "150=0", "151=1.00", "6=0.00", "14=0.00")
	s.Require().Nil(err)

	// trade execution
	s.srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.21679716,12000,"MARKET",12000,1,-0.39712904,"USD"]]`)

	// assert FIX execution report PARTIAL FILL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 3)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.2168", "39=1", "54=1", "55=tBTCUSD", "150=1", "151=0.7832", "6=12000.00", "14=0.2168")
	s.Require().Nil(err)

	// trade execution
	s.srvWs.Send(OrdersClient, `[0,"tu",[1,"tBTCUSD",1514909325593,1234567,0.78320284,12000,"MARKET",12000,1,-0.39712904,"USD"]]`)

	// assert FIX execution report FULL FILL
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 4)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=8", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "20=3", "32=0.7832", "39=2", "54=1", "55=tBTCUSD", "150=2", "151=0.0000", "6=12000", "14=1.000")
	s.Require().Nil(err)

	// attempt to cancel order
	cxl := fix42cxl.New(field.NewOrigClOrdID("555"),
		field.NewClOrdID("556"),
		field.NewSymbol("BTCUSD"),
		field.NewSide(enum.Side_BUY),
		field.NewTransactTime(time.Now()))
	err = session.Send(cxl)
	s.Require().Nil(err)

	// assert cancel req
	msg, err = s.srvWs.WaitForMessage(OrdersClient, 2)
	s.Require().Nil(err)
	today := time.Now().Format("2006-01-02")
	f := `[0,"oc",null,{"cid":555,"cid_date":"%s"}]`
	exp := fmt.Sprintf(f, today)
	s.Require().EqualValues(exp, msg)

	// publish cancel error
	s.srvWs.Send(OrdersClient, `[0,"n",[1521231457686,"oc-req",null,null,[null,null,555,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,null,0,null,null,null,null,null,null,null,null],null,"ERROR","Order not found."]]`)
	// assert FIX cancel reject
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 5)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=9", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "37=NONE", "11=555", "41=555", "39=8", "434=1", "102=1")
	s.Require().Nil(err)
}
