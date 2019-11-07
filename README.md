# NeoFS HTTP Gateway

NeoFS HTTP Gateway is example of tool that provides basic interactions with NeoFS.
You can download files from NeoFS Network using NeoFS Gateway. 

## Install

```go get -u github.com/nspcc-dev/neofs-gw```

## Configuration

```
# Environments:

GW_REQUEST_TIMEOUT=Duration             - timeout for request
GW_CONNECT_TIMEOUT=Duration             - timeout for connection
GW_LISTEN_ADDRESS=host:port             - address to listen connections
GW_NEOFS_NODE_ADDR=host:port            - address of NeoFS Node
GW_KEEPALIVE_TIME=Duration              - аfter a duration of this time if the client doesn't see any activity
it pings the server to see if the transport is still alive. 
GW_KEEPALIVE_TIMEOUT=Duration           - after having pinged for keepalive check, the client waits for a duration
of Timeout and if no activity is seen even after that the connection is closed
GW_KEEPALIVE_PERMIT_WITHOUT_STREAM=Bool - if true, client sends keepalive pings even with no active RPCs.
If false, when there are no active RPCs, Time and Timeout will be ignored and no keepalive pings will be sent.
```
