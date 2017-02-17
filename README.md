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

## Sessions

The current plan is to have one FIX instance per config file/port to enable restarting
single sessions in the future.

# Authentication

Authentication is problematic given the 
- FIX 4.4 allows username/password, FIX 4.2 does not
- possibly use RawData field in

Current approach is to use the SenderCompID and TargetCompID.

- configuration file will contain a batch of X sessions (senderCompId + targetCompId). 
  Each senderCompId is an authentication token that will manually associated to a user on 
  bitfinex's backend side. 
  (websocket authentication message will just be `{ "event": "auth", "token": SEND_COMP_ID}` ). 
  The creation of the sessions happen at the startup of the application "onCreate"
  event. So for each session, as soon as the app starts, we connect the authenticated 
  websocket and map it to the sessionId.

## Issues

OrderCancelReject currently does not get passed the ClOrdID of the OrderCancelRequest because bitfinex doesn’t care about it and in the current state there’s no way to pass that ClOrdID along.
One solution could be to register handlers for different responses inside the e.g. OrderCancelRequest that gets called once the reply from bitfinex arrives and then removed after.
