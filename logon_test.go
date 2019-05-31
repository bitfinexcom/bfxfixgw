package main

//TestLogon assures the gateway service will authenticate a websocket connection when receiving a FIX Logon message with valid credentials.
func (s *gatewaySuite) TestLogon() {
	// assert FIX MD logon
	fixm, err := s.fixMd.WaitForMessage(s.MarketDataSessionID, 1)
	s.Require().Nil(err)

	err = s.checkFixTags(fixm, "35=A", "49=BFXFIX", "56=EXORG_MD")
	s.Require().Nil(err)

	// assert FIX order logon
	fixm, err = s.fixOrd.WaitForMessage(s.OrderSessionID, 1)
	s.Require().Nil(err)

	err = s.checkFixTags(fixm, "35=A", "49=BFXFIX", "56=EXORG_ORD")
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
}

//TestLogonNoCredentials assures the gateway service will only authenticate a websocket connection when receiving a FIX Logon message with valid credentials.
func (s *gatewaySuite) TestLogonNoCredentials() {
	s.TearDownTest()
	oldSettings := s.settings
	defer func() { s.settings = oldSettings }()
	s.settings = mockFixSettings{FixVersion: s.settings.FixVersion}
	s.SetupTest()

	// give clients an opportunity to connect, but they should NOT be established
	err := s.srvWs.WaitForClientCount(2)
	s.Require().NotNil(err)

	fixm, err := s.fixMd.WaitForMessage(s.MarketDataSessionID, 1)
	s.Require().Nil(err)

	// expect malformed logon
	err = s.checkFixTags(fixm, "35=A", "49=BFXFIX", "56=EXORG_MD")
	s.Require().Nil(err)

	// logon reject prevents websocket connections
	err = s.srvWs.WaitForClientCount(0)
	s.Require().Nil(err)

	// TODO assert reject?
}

func (s *gatewaySuite) TestLogonInvalidCredentials() {
	// TODO assert reject?
}
