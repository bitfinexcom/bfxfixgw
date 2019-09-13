[![Build Status](https://api.travis-ci.org/bitfinexcom/bfxfixgw.png?branch=master)](https://travis-ci.org/bitfinexcom/bfxfixgw.svg?branch=master)

# Bitfinex FIX Gateway

The Bitfinex FIX gateway uses [QuickFix/go](https://github.com/quickfixgo/quickfix) to implement FIX connectivity. [The Bitfinex Go API](https://github.com/bitfinexcom/bitfinex-api-go) is used to manage the [Bitfinex API websocket](https://bitfinex.readme.io/docs) connection, which ultimately uses [Gorilla](https://github.com/gorilla/websocket).

## Build

### Go Environment

If one does not already exist, install a Go environment by following [these instructions](https://golang.org/doc/install).  If GOPATH is set to ~/go and the bfxfixgw repository is cloned to ~/go/src/github.com/bitfinexcom/bfxfixgw, then the following simple go command is all that is needed if issued from the ~/go directory.  Both bfxfixgw and fix_client will be installed to ~/go/bin after issuing the command.

Obtain all additional sources, build, and install:

```bash
go get ./...
```

### Building Source Changes

Local bfxfixgw source changes can be built by issuing the following commands from the ~/go/src/github.com/bitfinexcom/bfxfixgw directory.  The install command will copy the latest version of bfxfixgw and fix_client to ~/go/bin.

Test:

```bash
make check
```

Linter:

```bash
make lint
```

Build:

```bash
make build
```

And install binaries:

```bash
make install
```

Or do all above steps:

```bash
make all
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

The market data FIX service does not support resend requests or replaying FIX messages. When connecting to the market data FIX endpoint, a FIX initiator must have the correct sequence number, or a lower than expected sequence number.

The order routing service strictly tracks sequence numbers and does support message storage. A FIX initiator can send `ResetSeqNumFlag=Y` on Logon to reset session sequence numbers.

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

### Gateway Startup

To startup the gateway in verbose mode (-v) with both order routing and market data endpoints (staging configuration) run the following command:

```bash
FIX_SETTINGS_DIRECTORY=~/go/src/github.com/bitfinexcom/bfxfixgw/conf/integration_test/service ~/go/bin/bfxfixgw -v -orders -ordcfg "orders_fix42.cfg" -md -mdcfg "marketdata_fix42.cfg" -rest "https://api.bitfinex.com/v2/" -ws "wss://api.bitfinex.com/ws/2"
```

## Authentication

FIX session information must be obtained prior to a FIX client establishing a connection.  The pre-determined TargetCompID, SenderCompID, and FIX version strings should be configured in the FIX client configuration.

Once sessions are configured, a FIX client can authenticate by adding the Bitfinex User ID, API key, and API secret into the FIX `35=A Logon` message body:

| Field 				| FIX Tag # | Description 		|
|-----------------------|-----------|-------------------|
| BfxApiKey 			| 20000		| User's API Key	|
| BfxApiSecret 			| 20001		| User's API Secret	|
| BfxUserID 			| 20002		| User's Bfx ID		|

Optionally, a user can request all session-level orders be canceled when the FIX session is disconnected by setting the following FIX tag `8013=Y`:

| Field 				| FIX Tag # | Description 		|
|-----------------------|-----------|-------------------|
| CancelOnDisconnect	| 8013		| Cancel session orders on FIX disconnect |

---
**Note:**
FIX clients should use separate API keys for market data and order routing FIX endpoints.

---

These tags are supported by the gateway's default [data dictionary](spec/Bitfinex_FIX42.xml).  An example staging logon message (`SOH` replaced with `|`):

```
8=FIX.4.2|9=186|35=A|34=1|49=EXORG_ORD|52=20180416-18:27:47.541|56=BFXFIX|20000=U83q9jkML2GVj1fVxFJOAXQeDGaXIzeZ6PwNPQLEXt4|20001=77SWIRggvw0rCOJUgk9GVcxbldjTxOJP5WLCjWBFIVc|20002=connamara|98=0|108=30|10=117|
```

## Testing using fix_client

The project includes a test client utility called fix_client.  fix_client is a simple gateway client that demonstrates a subset of gateway functionality.  The client currently supports sending and canceling orders via the gateway.  Simply run the client, issue a root command (either nos or cxl), and then provide additional request parameters as prompted.

### Configuring fix_client

The fix_client takes a quickfix configuration file located here ~/go/src/github.com/bitfinexcom/bfxfixgw/conf/integration_test/client/orders_fix42.cfg.  To successfully connect to BFX using the gateway, you must change the following parameters to match the API access of your account.  Please note the security risk of adding credentials to this file in plain text.  You may want to remove these values after you are done with testing using fix_client

```
ApiKey=[User's API Key]  
ApiSecret=[User's API Secret]  
BfxUserID=[User's Bfx ID]  
```

### Running fix_client Example

Here is an example of using fix_client to place a market order.  All outgoing and incoming FIX traffic is logged.  You can decode the FIX manually or using your favorite FIX parser.  Please be careful to avoid sending FIX logs that contain private keys to online parsers.  Some logging has been stripped out for clarity and user input is **bold**.  

>**./fix_client -cfg ../src/github.com/bitfinexcom/bfxfixgw/conf/integration_test/client/orders_fix42.cfg**  
Enter command:  
**nos**  
-> New Order Single  
Enter ClOrdID (integer):  
**1**  
Enter symbol:  
**tBTCUSD**  
Enter order type:  
**market**  
Enter qty:  
**0.05**  
Enter side:  
**buy**  
Options? (hidden, postonly, fok): [FIX Client] MockFix.ToApp (outgoing): 8=FIX.4.2|9=126|35=D|34=547|49=EXORG_ORD|52=20180504-16:45:04.697|56=BFXFIX|11=1|21=3|38=0.0500|40=1|54=1|55=tBTCUSD|60=20180504-16:45:03.518|10=249|  
[FIX Client] MockFix.FromApp (incoming): 8=FIX.4.2|9=221|35=8|34=550|49=BFXFIX|52=20180504-16:45:04.804|56=EXORG_ORD|1=connamara|6=0.00|11=1|14=0.0000|17=43a98b1d-0312-41cf-9678-33e268e9bff9|20=3|32=0.0000|37=1149703695|38=0.0500|39=0|40=1|54=1|55=tBTCUSD|59=1|150=0|151=0.0500|10=007|  
<2018-05-04 16:45:05.823298994 +0000 UTC, FIX.4.2:EXORG_ORD->BFXFIX, incoming>  
[FIX Client] MockFix.FromApp (incoming): 8=FIX.4.2|9=250|35=8|34=551|49=BFXFIX|52=20180504-16:45:05.818|56=EXORG_ORD|1=connamara|6=850.00|11=1|12=0.0001|13=3|14=0.0500|17=1cfb00fc-9ea6-4dca-8d9f-a5cce1cc0959|20=3|31=850.0000|32=0.0500|37=1149703695|38=0.0500|39=2|40=1|54=1|55=tBTCUSD|59=1|150=2|151=0.0000|10=182  

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
8=FIX.4.2|9=168|35=W|34=3|49=BFXFIX|52=20180417-19:40:21.294|56=EXORG_MD|22=8|48=tBTCUSD|55=tBTCUSD|262=req-tBTCUSD|268=2|269=0|270=1663.9000|271=0.0400|269=1|270=1670.8000|271=0.1000|10=242|
```

Receive FIX `35=X` market data incremental update (for the first tBTCUSD request):

```
8=FIX.4.2|9=140|35=X|34=5|49=BFXFIX|52=20180417-19:41:48.474|56=EXORG_MD|262=req-tBTCUSD|268=1|279=2|269=1|55=tBTCUSD|48=tBTCUSD|22=8|270=0.0000|271=1.0000|10=185|
```

Receive FIX `35=X` trade incremental update (for the first tBTCUSD request):

```
8=FIX.4.2|9=143|35=X|34=5|49=BFXFIX|52=20180417-21:25:27.455|56=EXORG_MD|262=req-tBTCUSD|268=1|279=0|269=2|55=tBTCUSD|48=tBTCUSD|22=8|270=1671.0000|271=0.1000|10=081|
```

Receive FIX `35=AP` wallet snapshot and/or update

```
8=FIX.4.2|9=190|35=AP|34=2|49=BFXFIX|52=20190801-19:29:05.679|56=EXORG_ORD|1=user123|15=fUSD|581=exchange|715=20190801|721=fd96bcd8-0133-4740-a1e1-044feddd1be7|730=1123.4500|731=1|734=1234.5600|746=10.0000|10=099|
```

Receive FIX `35=AP` balance update

```
8=FIX.4.2|9=184|35=AP|34=2|49=BFXFIX|52=20190801-19:31:26.318|56=EXORG_ORD|1=user123|15=all|581=balance|715=20190801|721=fcefabf1-13b2-4f06-8015-521109c640de|730=12.3400|731=1|734=123.4500|746=0.0000|10=139|
```

## Order Routing

Order routing can be enabled with the `-ord` and `-ordcfg` flags on startup.

The following table lists Bitfinex order type support in the FIX gateway:

| Order Type	| FIX 4.2	| FIX 4.4	| FIX 5.0	|
|---------------|:---------:|:---------:|:---------:|
| Market		| ✔			| ✔			| ✔			|
| Limit			| ✔			| ✔			| ✔			|
| Stop			| ✔			| ✔			| ✔			|
| Stop Limit	| ✔			| ✔			| ✔			|
| Trailing Stop | ✔			| ✔			| ✔			|

Below is a table of various Bitfinex order features and their FIX `35=D NewOrderSingle` tags:

| Bitfinex Order Feature 	| FIX Tag				| FIX Tag Value					|
|---------------------------|-----------------------|-------------------------------|
| Hidden					| DisplayMethod (1084)	| Undisclosed (4)				|
| Post-Only<sup>*</sup>		| ExecInst (18)			| Participate don't initiate (6)|
| One Cancels Other (OCO)   | ContingencyType (1385)| One Cancels the Other (1)     |
| Fill or Kill				| TimeInForce (59)		| Fill or Kill (4)				|
| Good till Date			| TimeInForce (59)		| Good till Date (6)			|
| Good till Date			| ExpireTime (126)		| Example: 2006-01-02 15:04:05	|
| Margin<sup>*</sup>        | CashMargin (544)		| Margin Open (2)				|
| Leverage          		| Leverage (20005)		| Example: 10 (INTEGER)     	|

<sup>*</sup> Post-Only orders are considered to have a Good-till-Cancel time in force.  

<sup>*</sup> Margin order execution reports respond with CashMargin (544) = Margin Close (3)

For a trailing stop order:

| Trailing Stop Feature		| FIX Tag				| FIX Tag Value						|
|---------------------------|-----------------------|-----------------------------------|
| Order Type				| OrdType (40)			| Stop (3) or Stop Limit (4)	|
| Execute as Trailing Peg	| ExecInst (18)			| Primary Peg (R)				|
| Trailing Peg Value		| PegOffsetValue (211)	| Price<sup>*</sup>				|

<sup>*</sup> The trailing stop price should be in the same units as a Price (44) or StopPx (99).

e.g. For a trailing stop order where the stop should trail the market by $5.25, the following FIX tags should be set:

| Field				| Tag	| Value		|
|-------------------|-------|-----------|
| OrdType			| 40	| 4			|
| ExecInst			| 18	| R			|
| PegOffsetValue	| 211	| 5.25		|

### Examples

Send limit new order single:

```
8=FIX.4.2|9=142|35=D|34=34|49=EXORG_ORD|52=20180417-22:28:26.326|56=BFXFIX|11=2000|21=3|38=0.1000|40=2|44=20000.0000|54=2|55=tBTCUSD|60=20180417-22:28:26.326|10=208|
```

Receive FIX `35=8` execution report for working order:

```
8=FIX.4.2|9=232|35=8|34=38|49=BFXFIX|52=20180417-22:28:26.555|56=EXORG_ORD|1=connamara|6=0.00|11=2000|14=0.0000|17=935a0300-2f34-4908-9e03-6b899f9718c6|20=3|32=0.0000|37=1149698709|38=0.1000|39=0|40=2|44=20000.0000|54=2|55=tBTCUSD|150=0|151=0.1000|10=097|
```

Send market new order single:

```
8=FIX.4.2|9=128|35=D|34=39|49=EXORG_ORD|52=20180417-22:30:34.310|56=BFXFIX|11=2002|21=3|38=1.5000|40=1|54=2|55=tBTCUSD|60=20180417-22:30:34.310|10=059|
```

Receive FIX `35=8` execution report for a partial fill:

```
8=FIX.4.2|9=249|35=8|34=45|49=BFXFIX|52=20180417-22:30:35.100|56=EXORG_ORD|1=connamara|6=1663.90|11=2002|12=0.1331|13=3|14=0.0400|17=f4d8af6e-04ce-447b-80ed-3e82d1476274|20=3|31=1663.9000|32=0.0400|37=1149698710|38=1.5000|39=1|40=1|54=2|55=tBTCUSD|150=1|151=1.4600|10=163|
```

Receive the alst FIX `35=8` execution report for a full fill:

```
8=FIX.4.2|9=249|35=8|34=48|49=BFXFIX|52=20180417-22:30:35.135|56=EXORG_ORD|1=connamara|6=1662.70|11=2002|12=0.3327|13=3|14=1.5000|17=6ee60005-4e99-4e6d-aa51-3aa617489dc7|20=3|31=1663.5000|32=0.1000|37=1149698710|38=1.5000|39=2|40=1|54=2|55=tBTCUSD|150=2|151=0.0000|10=112|
```

Cancel working limit order:

```
8=FIX.4.2|9=116|35=F|34=36|49=EXORG_ORD|52=20180417-22:29:11.115|56=BFXFIX|11=2001|41=2000|54=2|55=tBTCUSD|60=20180417-22:29:11.115|10=051|
```

Receive FIX `35=8` pending cancel acknowledgement:
```
8=FIX.4.2|9=296|35=8|34=40|49=BFXFIX|52=20180417-22:29:11.290|56=EXORG_ORD|1=connamara|6=0.00|11=2000|14=0.0000|17=d067ff27-4182-454f-8c51-bd08c09b5730|20=3|37=1149698709|38=0.1000|39=6|40=2|44=20000.0000|54=2|55=tBTCUSD|58=Submitted for cancellation; waiting for confirmation (ID: 1149698709).|150=6|151=0.1000|10=112|
```

Receive FIX `35=8` cancel acknowledgement:

```
8=FIX.4.2|9=244|35=8|34=41|49=BFXFIX|52=20180417-22:29:11.305|56=EXORG_ORD|1=connamara|6=0.00|11=2000|14=0.0000|17=a674d1b4-214e-408a-8cc1-fa364ecd8d97|20=3|32=0.0000|37=1149698709|38=0.1000|39=4|40=2|44=20000.0000|54=2|55=tBTCUSD|58=CANCELED|150=4|151=0.0000|10=112|
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

## Average prices & working quantities for out-of-session working orders

Orders placed outside of the FIX trading session may sometimes have incorrect filled quantities (FIX tag 14 `CumQty`), remaining quantities (FIX tag 151 `LeavesQty`), and/or average prices (FIX tag 6 `AvgPx`). The gateway uses an order's individual executions to calculate average prices, filled quantities, and remaining quantities on demand. The gateway does not receive working order execution details in order snapshots on client logon.  Therefore, the gateway will attempt to go out-of-band, using the authenticated REST API, to fetch execution details for working orders.  Out of band requests use the same API key as the orders websocket connection, and may suffer from internet routing race conditions.  Occasionally the order trades endpoint will unexpectedly return an empty set, therefore execution details may not always be available to the gateway.

Internet routing race conditions could be solved with a future enhancement to the gateway to use yet another (third) API key for authenticated out-of-band REST requests.

## Execution reports out of order

To preserve fee information, `tu` API messages are used to populate execution reports.  However, the API publishes `tu` messages out of order, so corresponding ERs may also be out of order.

## Immediate or Cancel collapses into Fill or Kill time in force

If an order is sent with an Immediate or Cancel time in force, the order will be mapped as a Bitfinex fill or kill limit order. Corresponding execution reports will indicate the order was placed as a fill or kill limit and not an IOC limit order.

## Unsolicited trailing stop Execution Report missing trailing peg

If a trailing stop order was placed outside of the FIX session, a `39=0 NEW` Execution Report will be missing the trailing stop peg price.  The Bitfinex API currently does not return trailing stop peg prices on order new notification acknowledgements, but instead lists the calculated stop price, which is included in tag `99 StopPx` on the `39=0 NEW` ExecutionReport.  Subsequent ExecutionReports related to the unsolicited trailing stop may also be missing the peg price until the gateway's cache is updated from the Bitfinex API.
