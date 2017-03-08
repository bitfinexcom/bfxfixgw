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

## Issues

`OrderCancelReject` currently does not get passed the `ClOrdID` of the `OrderCancelRequest`
because bitfinex doesn’t care about it and in the current state there’s no way
to pass that `ClOrdID` along.
One solution could be to register handlers for different responses inside the 
e.g. `OrderCancelRequest` that gets called once the reply from bitfinex arrives
and then removed after.
