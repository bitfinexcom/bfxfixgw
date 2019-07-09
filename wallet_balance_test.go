package main

//TestNewOrderSingleBuyLimitFill assures the gateway service will publish an OrderNew websocket message when receiving a FIX42 NewOrderSingle
func (s *gatewaySuite) TestWalletSnapshotUpdate() {
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

	// service publish wallet snapshot
	s.srvWs.Send(OrdersClient, `[0,"ws",[["exchange", "fUSD", 1234.56, 10.0, 1123.45]]]`)
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 2)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=AP", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "581=exchange", "15=fUSD", "730=1123.4500", "746=10.0000", "734=1234.5600")
	s.Require().Nil(err)

	// service publish wallet update
	s.srvWs.Send(OrdersClient, `[0,"wu",["exchange", "fUSD", 2234.56, 20.0, 2123.45]]`)
	fix, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 3)
	s.Require().Nil(err)
	err = s.checkFixTags(fix, "35=AP", "49=BFXFIX", "56=EXORG_ORD", "1=user123", "581=exchange", "15=fUSD", "730=2123.4500", "746=20.0000", "734=2234.5600")
	s.Require().Nil(err)
}
