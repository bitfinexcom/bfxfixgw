# Bitfinex FIX Gateway

The Bitfinex FIX gateway uses [QuickFix/go](https://github.com/quickfixgo/quickfix) to implement FIX connectivity. [The Bitfinex Go API](https://github.com/bitfinexcom/bitfinex-api-go) is used to manage the [Bitfinex API websocket](https://bitfinex.readme.io/docs) connection, which ultimately uses [Gorilla](https://github.com/gorilla/websocket).

## Build

First obtain all sources:

```bash
go get ./...
```

Build:

```bash
go build ./...
```

Run tests:

```bash
go test ./...
```

And install binaries:

```bash
go install ./...
```

## Configuration

The FIX gateway can operate in order routing mode, market data mode, or both.  Order routing & market data FIX endpoints must be separate with distinct session IDs.

### Environment

- `DEBUG=1` to enable debug logging
- `FIX_SETTINGS_DIRECTORY=./config` to read the configs from the given directory, `./config`
  is the default directory.

### Sessions

Sessions must be known to the FIX gateway prior to startup.  A FIX gateway can manage any number of sessions.  Each FIX session will create 1 websocket proxy connection.

#### Sequence Numbers

The market data service does not support resend requests or replaying FIX messages. When connecting to the market data FIX endpoint, a FIX initiator must either have the correct sequence number or a lower than expected sequence number.

The order routing service strictly tracks sequence numbers and does support message storage.

### FIX Configuration Examples

Example service FIX session configuration for a market data service:

```
[DEFAULT]
SenderCompID=BFXFIX
ResetOnLogon=Y
ReconnectInterval=60
FileLogPath=tmp/md_service/log
SocketAcceptPort=5001
StartTime=00:05:00
StartDay=Sun
EndTime=00:00:00
EndDay=Sun

[SESSION]
TargetCompID=EXORG_MD
BeginString=FIX.4.2
DefaultApplVerID=FIX.4.2
HeartBtInt=30
```

Example service FIX session configuration for an order routing service:

```
[DEFAULT]
SenderCompID=BFXFIX
ReconnectInterval=60
FileLogPath=tmp/ord_service/log
FileStorePath=tmp/ord_service/data
SocketAcceptPort=5002
StartTime=00:05:00
StartDay=Sun
EndTime=00:00:00
EndDay=Sun

[SESSION]
TargetCompID=EXORG_ORD
BeginString=FIX.4.2
DefaultApplVerID=FIX.4.2
HeartBtInt=30
```

### Startup

To startup the gateway with both order routing and market data endpoints (staging configuration):

```bash
FIX_SETTINGS_DIRECTORY=conf/integration_test/service/ bfxfixgw.exe -orders -ordcfg orders_fix42.cfg -md -mdcfg marketdata_fix42.cfg -ws wss://dev-prdn.bitfinex.com:2998/ws/2 -rest https://dev-prdn.bitfinex.com:2998/v2/
```

## Authentication

FIX session information must be obtained prior to a FIX client establishing a connection.  The pre-determined TargetCompID, SenderCompID, and FIX version strings should be configured in the FIX client configuration.

Once sessions are configured, a FIX client can authenticate by adding the following FIX fields into the `35=A Logon` message body:

| Field 		| FIX Tag # | Description 		|
|---------------|-----------|-------------------|
| BfxApiKey 	| 20000		| User's API Key	|
| BfxApiSecret 	| 20001		| User's API Secret	|
| BfxUserID 	| 20002		| User's Bfx ID		|

These tags are supported by the gateway's default [data dictionary](spec/Bitfinex_FIX42.xml).  An example staging logon message (`SOH` replaced with `|`):

```
8=FIX.4.2|9=186|35=A|34=1|49=EXORG_ORD|52=20180416-18:27:47.541|56=BFXFIX|20000=U83q9jkML2GVj1fVxFJOAXQeDGaXIzeZ6PwNPQLEXt4|20001=77SWIRggvw0rCOJUgk9GVcxbldjTxOJP5WLCjWBFIVc|20002=connamara|98=0|108=30|10=117|
```

## Market Data Distribution

The FIX gateway service may be configured to distribute market data. Starting the process with `-md` will enable market data distribution, configured by the `-mdcfg` flag.

### Examples

Subscribe to `tBTCUSD` top-of-book Precision0 updates:

```
8=FIX.4.2|9=111|35=V|34=2|49=EXORG_MD|52=20180417-19:40:17.467|56=BFXFIX|146=1|55=tBTCUSD|262=req-tBTCUSD|263=1|264=1|20003=P0|10=167|
```

Subscribe to `tETHUSD` full raw book updates at 25 price levels:

```
8=FIX.4.2|9=112|35=V|34=3|49=EXORG_MD|52=20180417-19:46:44.594|56=BFXFIX|146=1|55=tETHUSD|262=req-tETHUSD|263=1|264=25|20003=R0|10=248|
```

Receive FIX `35=W` book snapshot (for the first tBTCUSD request):

```
TODO
```

Receive FIX `35=W` trade snapshot (for the first tBTCUSD request):

```

```

Receive FIX `35=X` incremental update (for the first tBTCUSD request):

```
TODO
```

## Order Routing

Order routing can be enabled with the `-ord` and `-ordcfg` flags on startup.

### Examples

Send limit new order single:

```
TODO
```

Send market new order single:

```
TODO
```

Receive FIX `35=8` execution report for working order:

```
TODO
```

Receive FIX `35=8` execution report for a partial or full fill:

```
TODO
```

Cancel working limit order:

```
TODO
```

Receive FIX `35=8` cancel acknowledgement:

```
TODO
```

## Order State Details

When receiving a Bitfinex Order update object (on, ou, oc), the following tables demonstrate rules for mapping FIX tag `39 OrdStatus`:

### Order Update Status Mappings

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

### Synthetic Order State Message Mappings

`on-req` generally maps to PENDING NEW, with an exception for market orders, which do not receive subsequent `on` ack working messages.

| BFX Order Type	| Incoming BFX Message	| FIX OrdStatus Code	| Order Status		| Notes	|
|-------------------|-----------------------|-----------------------|-------------------|-------|
| EXCHANGE MARKET	| n (on-req)			| 0					| NEW	| Market orders do not receive `on` messages. |
| EXCHANGE LIMIT	| n (on-req)			| 0						| PENDING NEW		| |
| EXCHANGE LIMIT	| oc					| 4						| CANCELED			| `oc` objects are also received for terminal order states, such as fills, in which case an `oc` will generate no FIX message |
| EXCHANGE LIMIT	| ou					| Depends on status		| Depends on status	| |

## Troubleshooting

Below are a few common issues with simple procedures to resolve runtime problems:

#### FIX client won't log on

If the session has been rolled over or restarted, a FIX initiator may have a higher sequence number than its acceptor, which is an error condition.  Simply reset the FIX initiator's sequence number (deleting the sequence store file in QuickFIX works) and the initiator should no longer disconnect on logon.

#### FIX logs on but does not process requests

Ensure the correct endpoint is configured for use. (i.e. MarketDataRequests should be sent to the FIX Market Data endpoint, and NewOrderSingle messages should be sent to the order routing endpoint).

# Issues

## Average price on restart

The gateway calculates average fill price based on executions the gateway has received and stored in working memory.

If the gateway is restarted while a client's order is partially filled, but still working, the average fill prices will only reflect fills subsequent to the gateway's restart.

To fix this issue, the gateway should fetch execution information for each order in the order snapshot received when logging a user onto the Bitfinex API, which it currently does not do.