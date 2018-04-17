# bfxfixgw

We are going to use [quickfixgo](https://github.com/quickfixgo/quickfix) to implement 
the FIX side of the proxy and [gorilla’s websocket](https://github.com/gorilla/websocket) 
package for the communication between the proxy and [bitfinex’s websocket API](https://bitfinex.readme.io/docs).

# Configuration

The configuration is supplied to quickfixgo via a simple `io.Reader`, which means
that the actual config file can be stored just about anywhere. The problem is 
that right now it seems like quickfixgo does not offer a method to reload the 
global configuration, meaning that adding a new session would require a stop/start 
of all sessions. One possible remedy for that with the current quickfixgo library 
could be running instance per session, i.e. each user gets their own port to
connect to.

At some point in the future extending the quickfixgo library to allow a graceful
reload of the config could be considered.

## Environment

- `DEBUG=1` to enable debug logging
- `FIX_SETTINGS_DIRECTORY=./config` to read the configs from the given directory, `./config`
  is the default directory.

Staging startup from source:

```bash
FIX_SETTINGS_DIRECTORY=conf/integration_test/service/ bfxfixgw.exe -orders -ordcfg orders_fix42.cfg -md -mdcfg marketdata_fix42.cfg -ws wss://dev-prdn.bitfinex.com:2998/ws/2 -rest https://dev-prdn.bitfinex.com:2998/v2/
```

## Sessions

The current plan is to have one FIX instance per config file/port to enable restarting
single sessions in the future.

The creation of the sessions happen at the startup of the application "onCreate"
event. So for each session, as soon as the app starts, we connect the authenticated 
websocket and map it to the sessionId.

# Authentication

Authentication is problematic given that FIX 4.4 allows username/password, FIX 
4.2 does not, which means that there's no consisten way of handling the authentication.
One possibility would be to use the RawData field, but many clients don't seem to
support that.
The current approach is to use the `SenderCompID` and `TargetCompID`. The 
`SenderCompID` is the key for the API and the `TargetCompID` is the secret, e.g.
one session in the cfg file for the proxy/server might look like this:

```
[SESSION]
SenderCompID=wGqfsyrEi2c1eD4c1E9BHAXCL50rE4j2wqjNQjq4DAh
TargetCompID=8NGQqizVUE5e3benJklHpB33FSFHGb64U49h1rGBhHZ
BeginString=FIX.4.4
```

The issues with using `SenderCompID` and `TargetCompID` are the misuse of the
protocol and both fields only being protected by TLS, i.e. without TLS they are
transmitted in plaintext over the wire, because they're part of the Header, which
is always unencrypted.

It was also proposed to have a configuration file containing a batch of X sessions
(senderCompId + targetCompId), where each senderCompId is an authentication token
that will manually associated to a user on bitfinex's backend side, so websocket
authentication message will just be `{ "event": "auth", "token": SEND_COMP_ID}`. 

# Market Data Distribution

The FIX gateway service may be configured to distribute market data. Starting the process with `-md` will enable market data distribution, configured by the `-mdcfg` flag.

# Order Routing

# Order State Details

When receiving a Bitfinex Order update object (on, ou, oc), the following tables demonstrate rules for mapping tag 39 OrdStatuses.

## Bitfinex order update status mappings

| BFX Order State 	| FIX OrdStatus Code 	| Order Status 		|
|-------------------|-----------------------|-------------------|
| ACTIVE			| 0						| NEW				|
| EXECUTED			| 2						| FILLED			|
| PARTIALLY FILLED	| 1						| PARTIALLY FILLED	|
| CANCELED			| 4						| CANCELED			|

Executions are received as `te` TradeExecution messages and `tu` TradeUpdate messages.  TradeExecution messages come first, but generally omit the order type, original price, fee, and some other fields.  The gateway processes `tu` TradeUpdate messages as executions.  When receiving a TradeUpdate, MsgType `8` ExecutionReports are generated following these rules:

- 1 TradeUpdate message will create 1 ExecutionReport
- Tag 37 OrderID is derived from the `tu` server-assigned ID
- An order's CID is mapped to a tag 11 ClOrdID
- The gateway maintains an in-memory cache of ClOrdID -> order information, including:
	- Original order details (symbol, account, price, quantity, type, side)
	- Calculated state details (average fill price, total open quantity, total filled quantity)
	- Executions related to the original order
	- Cancel details related to a ClOrdID (original order ID, symbol, account, ClOrdID, and side)
- When receiving order state updates (rejection, fill, cancel acknowledgement), the cache must be referenced to provide FIX-required details
- When receiving a TradeUpdate, if cached details indicate the incoming TradeUpdate would fully fill the order, the gateway will publish an ExecutionReport with an OrdStatus of FILLED.

## Synthetic order state message mappings

`on-req` generally maps to PENDING NEW, with an exception for market orders, which do not receive subsequent `on` ack working messages.

| BFX Order Type	| Incoming BFX Message	| FIX OrdStatus Code	| Order Status		| Notes	|
|-------------------|-----------------------|-----------------------|-------------------|-------|
| EXCHANGE MARKET	| n (on-req)			| A & 0					| PENDING NEW & NEW	| Market orders do not receive `on` messages.  The first successful acknowledgement publishes both PENDING NEW and NEW execution reports. |
| EXCHANGE LIMIT	| n (on-req)			| A						| PENDING NEW		| |
| EXCHANGE LIMIT	| on					| 0						| NEW				| |
| EXCHANGE LIMIT	| oc					| 4						| CANCELED			| |
| EXCHANGE LIMIT	| ou					| Depends on status		| Depends on status	| |

# Troubleshooting

Below are a few common issues with simple procedures to resolve runtime problems.

## FIX client won't log on

If the session has been rolled over or restarted, a FIX initiator may have a higher sequence number than its acceptor, which is an error condition.  Simply reset the FIX initiator's sequence number (deleting the sequence store file in QuickFIX works) and the initiator should no longer disconnect on logon.

## FIX logs on but does not process requests

Ensure the correct endpoint is configured for use. (i.e. MarketDataRequests should be sent to the FIX Market Data endpoint, and NewOrderSingle messages should be sent to the order routing endpoint).

# Issues

- Cached state details are lost on restart (must fetch account order & execution state on startup)
