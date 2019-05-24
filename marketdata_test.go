package main

import (
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	mdr "github.com/quickfixgo/fix42/marketdatarequest"
)

func newMdRequest(reqID, symbol string, depth int) *mdr.MarketDataRequest {
	mdreq := mdr.New(field.NewMDReqID(reqID), field.NewSubscriptionRequestType(enum.SubscriptionRequestType_SNAPSHOT_PLUS_UPDATES), field.NewMarketDepth(depth))
	nrsg := mdr.NewNoRelatedSymRepeatingGroup()
	nrs := nrsg.Add()
	nrs.Set(field.NewSymbol(symbol))
	mdreq.SetNoRelatedSym(nrsg)
	return &mdreq
}

func (s *gatewaySuite) TestMarketData() {
	// assert FIX MD logon
	fix, err := s.fixMd.WaitForMessage(MarketDataSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	s.Require().Nil(err)
	// assert FIX order logon
	fix, err = s.fixOrd.WaitForMessage(OrderSessionID, 1)
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

	// assert FIX MD logon
	fix, err = s.fixMd.WaitForMessage(MarketDataSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_MD")
	s.Require().Nil(err)
	fix, err = s.fixOrd.WaitForMessage(OrderSessionID, 1)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=A", "49=BFXFIX", "56=EXORG_ORD")
	s.Require().Nil(err)

	// request market data
	req := newMdRequest("request-id-1", "tBTCUSD", 1)
	err = s.fixMd.Send(req)
	s.Require().Nil(err)

	// wait for ws to see requests
	msg, err = s.srvWs.WaitForMessage(MarketDataClient, 1)
	s.Require().Nil(err)
	// raw book precision, no frequency
	s.Require().EqualValues(`{"subId":"nonce2","event":"subscribe","channel":"book","symbol":"tBTCUSD","prec":"P0","freq":"F0","len":"1"}`, msg)

	msg, err = s.srvWs.WaitForMessage(MarketDataClient, 2)
	s.Require().Nil(err)
	s.Require().EqualValues(`{"subId":"nonce3","event":"subscribe","channel":"trades","symbol":"tBTCUSD"}`, msg)

	// ack book sub req
	s.srvWs.Send(MarketDataClient, `{"event":"subscribed","channel":"book","chanId":8,"symbol":"tBTCUSD","prec":"P0","freq":"F0","len":"1","subId":"nonce2","pair":"BTCUSD"}`)

	// ack trades sub req
	s.srvWs.Send(MarketDataClient, `{"event":"subscribed","channel":"trades","chanId":19,"symbol":"tBTCUSD","subId":"nonce3","pair":"BTCUSD"}`)

	// srv->client book snapshot
	s.srvWs.Send(MarketDataClient, `[8,[[1085.2,1,0.16337353],[1085,1,1],[1084.5,1,-0.0360446]]]`)

	// assert book snapshot
	fix, err = s.fixMd.WaitForMessage(MarketDataSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=W", "268=3", "269=0|270=1085.2000|271=0.1634|269=0|270=1085.0000|271=1.0000|269=1|270=1084.5000|271=0.0360", "48=tBTCUSD", "22=8")
	s.Require().Nil(err)

	// srv->client book update
	s.srvWs.Send(MarketDataClient, `[8,[1084,1,0.05246595]]`)

	// assert book update
	fix, err = s.fixMd.WaitForMessage(MarketDataSessionID, 3)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=X", "268=1", "279=0", "269=0", "48=tBTCUSD", "22=8", "271=0.0525")
	s.Require().Nil(err)

	// srv->client trade snapshot
	s.srvWs.Send(MarketDataClient, `[19,[[24165028,1516316211920,-0.05955414,1085.2],[24165027,1516316200519,-0.04440374,1085.2],[24165026,1516316189651,-0.0551028,1085.2]]]`)

	// do not publish public trade snapshot (35=W)

	// srv->client trade update
	s.srvWs.Send(MarketDataClient, `[19,[24165025,1516316086676,-0.05246595,1085.2]]`)

	fix, err = s.fixMd.WaitForMessage(MarketDataSessionID, 4)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=X", "268=1", "279=0", "269=2", "48=tBTCUSD", "22=8", "271=0.0525")
	s.Require().Nil(err)
}
