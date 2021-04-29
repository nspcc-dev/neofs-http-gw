# NeoFS HTTP Protocol Gateway

NeoFS HTTP Protocol Gateway bridges NeoFS internal protocol and HTTP standard.
- you can download one file per request from NeoFS Network
- you can upload one file per request into the NeoFS Network

## Installation

```go get -u github.com/nspcc-dev/neofs-http-gate```

Or you can call `make` to build it from the cloned repository (the binary will
end up in `bin/neofs-http-gw`).

### Notable make targets

```
dep          Check and ensure dependencies
image        Build clean docker image
dirty-image  Build dirty docker image with host-built binaries
fmts         Run all code formatters
lint         Run linters
version      Show current version
```

## Execution

HTTP gateway itself is not a NeoFS node, so to access NeoFS it uses node's
gRPC interface and you need to provide some node that it will connect to. This
can be done either via `-p` parameter or via `HTTP_GW_PEERS_<N>_ADDRESS` and
`HTTP_GW_PEERS_<N>_WEIGHT` environment variables (the gate supports multiple
NeoFS nodes with weighted load balancing).

These two commands are functionally equivalent, they run the gate with one
backend node (and otherwise default settings):
```
$ neofs-http-gw -p 192.168.130.72:8080
$ HTTP_GW_PEERS_0_ADDRESS=192.168.130.72:8080 neofs-http-gw
```

### Configuration

In general, everything available as CLI parameter can also be specified via
environment variables, so they're not specifically mentioned in most cases
(see `--help` also).

#### Nodes and weights

You can specify multiple `-p` options to add more NeoFS nodes, this will make
gateway spread requests equally among them (using weight 1 for every node):

```
$ neofs-http-gw -p 192.168.130.72:8080 -p 192.168.130.71:8080
```
If you want some specific load distribution proportions, use weights, but they
can only be specified via environment variables:

```
$ HTTP_GW_PEERS_0_ADDRESS=192.168.130.72:8080 HTTP_GW_PEERS_0_WEIGHT=9 \
  HTTP_GW_PEERS_1_ADDRESS=192.168.130.71:8080 HTTP_GW_PEERS_1_WEIGHT=1 neofs-http-gw
```
This command will make gateway use 192.168.130.72 for 90% of requests and
192.168.130.71 for remaining 10%.

#### Keys

By default gateway autogenerates key pair it will use for NeoFS requests. If
for some reason you need to have static keys you can pass them via `--key`
parameter. The key can be a path to private key file (as raw bytes), a hex
string or (unencrypted) WIF string. Example:

```
$ neofs-http-gw -p 192.168.130.72:8080 -k KxDgvEKzgSBPPfuVfw67oPQBSjidEiqTHURKSDL1R7yGaGYAeYnr
```

#### Binding and TLS

Gateway binds to `0.0.0.0:8082` by default and you can change that with
`--listen_address` option.

It can also provide TLS interface for its users, just specify paths to key and
certificate files via `--tls_key` and `--tls_certificate` parameters. Note
that using these options makes gateway TLS-only, if you need to serve both TLS
and plain text HTTP you either have to run two gateway instances or use some
external redirecting solution.

Example to bind to `192.168.130.130:443` and serve TLS there:

```
$ neofs-http-gw -p 192.168.130.72:8080 --listen_address 192.168.130.130:443 \
  --tls_key=key.pem --tls_certificate=cert.pem
```

#### HTTP parameters

You can tune HTTP read and write buffer sizes as well as timeouts with
`HTTP_GW_WEB_READ_BUFFER_SIZE`, `HTTP_GW_WEB_READ_TIMEOUT`,
`HTTP_GW_WEB_WRITE_BUFFER_SIZE` and `HTTP_GW_WEB_WRITE_TIMEOUT` environment
variables.

`HTTP_GW_WEB_STREAM_REQUEST_BODY` environment variable can be used to disable
request body streaming (effectively it'll make gateway accept file completely
first and only then try sending it to NeoFS).

`HTTP_GW_WEB_MAX_REQUEST_BODY_SIZE` controls maximum request body size
limiting uploads to files slightly lower than this limit.

#### NeoFS parameters

Gateway can automatically set timestamps for uploaded files based on local
time source, use `HTTP_GW_UPLOAD_HEADER_USE_DEFAULT_TIMESTAMP` environment
variable to control this behavior.

#### Monitoring and metrics

Pprof and Prometheus are integrated into the gateway, but not enabled by
default. To enable them use `--pprof` and `--metrics` flags or
`HTTP_GW_PPROF`/`HTTP_GW_METRICS` environment variables.

#### Timeouts

You can tune gRPC interface parameters with `--connect_timeout` (for
connection to node) and `--request_timeout` (for request processing over
established connection) options as well as `HTTP_GW_KEEPALIVE_TIME`
(peer pinging interval), `HTTP_GW_KEEPALIVE_TIMEOUT` (peer pinging timeout)
and `HTTP_GW_KEEPALIVE_PERMIT_WITHOUT_STREAM` environment variables.

gRPC-level checks allow gateway to detect dead peers, but it declares them
unhealthy at pool level once per `--rebalance_timer` interval, so check for it
if needed.

All timing options accept values with suffixes, so "15s" is 15 seconds and
"2m" is 2 minutes.

#### Logging

`--verbose` flag enables gRPC logging and there is a number of environment
variables to tune logging behavior:

```
HTTP_GW_LOGGER_FORMAT=string                     - Logger format
HTTP_GW_LOGGER_LEVEL=string                      - Logger level
HTTP_GW_LOGGER_NO_CALLER=bool                    - Logger don't show caller
HTTP_GW_LOGGER_NO_DISCLAIMER=bool                - Logger don't show application name/version
HTTP_GW_LOGGER_SAMPLING_INITIAL=int              - Logger sampling initial
HTTP_GW_LOGGER_SAMPLING_THEREAFTER=int           - Logger sampling thereafter
HTTP_GW_LOGGER_TRACE_LEVEL=string                - Logger show trace on level
```

## HTTP API provided

This gateway intentionally provides limited feature set and doesn't try to
substitute (or completely wrap) regular gRPC NeoFS interface. You can download
and upload objects with it, but deleting, searching, managing ACLs, creating
containers  and other activities are not supported and not planned to be
supported.

### Downloading

#### Requests

Basic downloading involves container and object ID and is done via GET
requests to `/get/$CID/$OID` path, like this:

```
$ wget http://localhost:8082/get/Dxhf4PNprrJHWWTG5RGLdfLkJiSQ3AQqit1MSnEPRkDZ/2m8PtaoricLouCn5zE8hAFr3gZEBDCZFe9BEgVJTSocY

```

There is also more complex interface provided for attribute-based downloads,
it's usually used to retrieve files by their names, but any other attribute
can be used as well. The generic syntax for it looks like this:

```/get_by_attribute/$CID/$ATTRIBUTE_NAME/$ATTRIBUTE_VALUE```

where `$CID` is a container ID, `$ATTRIBUTE_NAME` is the name of the attribute
we want to use and `ATTRIBUTE_VALUE` is the value of this attribute that the
target object should have.

If multiple objects have specified attribute with specified value, then the
first one of them is returned (and you can't get others via this interface).

Example for file name attribute:

```
$ wget http://localhost:8082/get_by_attribute/88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM/FileName/cat.jpeg
```

Some other user-defined attribute:

```
$ wget http://localhost:8082/get_by_attribute/Dxhf4PNprrJHWWTG5RGLdfLkJiSQ3AQqit1MSnEPRkDZ/Ololo/100500
```

An optional `download=true` argument for `Content-Disposition` management is
also supported (more on that below):

```
$ wget http://localhost:8082/get/Dxhf4PNprrJHWWTG5RGLdfLkJiSQ3AQqit1MSnEPRkDZ/2m8PtaoricLouCn5zE8hAFr3gZEBDCZFe9BEgVJTSocY?download=true

```

#### Replies

You get object contents in the reply body, but at the same time you also get a
set of reply headers generated using the following rules:
 * `Content-Length` is set to the length of the object
 * `Content-Type` is autodetected dynamically by gateway
 * `Content-Disposition` is `inline` for regular requests and `attachment` for
   requests with `download=true` argument, `filename` is also added if there
   is `FileName` attribute set for this object
 * `Last-Modified` header is set to `Timestamp` attribute value if it's
   present for the object
 * `x-container-id` contains container ID
 * `x-object-id` contains object ID
 * `x-owner-id` contains owner address
 * all the other NeoFS attributes are converted to `x-*` attributes (but only
   if they can be safely represented in HTTP header), for example `FileName`
   attribute becomes `x-FileName`

### Uploading

You can POST files to `/upload/$CID` path where `$CID` is container ID. The
request must contain multipart form with mandatory `filename` parameter. Only
one part in multipart form will be processed, so to upload another file just
issue new POST request.

Example request:

```
$ curl -F 'file=@cat.jpeg;filename=cat.jpeg' http://localhost:8082/upload/Dxhf4PNprrJHWWTG5RGLdfLkJiSQ3AQqit1MSnEPRkDZ
```

Chunked encoding is supported by the server (but check for request read
timeouts if you're planning some streaming). You can try streaming support
with large file piped through named FIFO pipe:

```
$ mkfifo pipe
$ cat video.mp4 > pipe &
$ curl --no-buffer -F 'file=@pipe;filename=catvideo.mp4' http://localhost:8082/upload/Dxhf4PNprrJHWWTG5RGLdfLkJiSQ3AQqit1MSnEPRkDZ
```

You can also add some attributes to your file using the following rules:
 * all "X-Attribute-*" headers get converted to object attributes with
   "X-Attribute-" prefix stripped, that is if you add "X-Attribute-Ololo:
   100500" header to your request the resulting object will get "Ololo:
   100500" attribute
 * "X-Attribute-NEOFS-*" headers are special, they're used to set internal
   NeoFS attributes starting with `__NEOFS__` prefix, for these attributes all
   dashes get converted to underscores and all letters are capitalized. For
   example, you can use "X-Attribute-NEOFS-Expiration-Epoch" header to set
   `__NEOFS__EXPIRATION_EPOCH` attribute
 * `FileName` attribute is set from multipart's `filename` if not set
   explicitly via `X-Attribute-FileName` header
 * `Timestamp` attribute can be set using gateway local time if using
   HTTP_GW_UPLOAD_HEADER_USE_DEFAULT_TIMESTAMP option and if request doesn't
   provide `X-Attribute-Timestamp` header of its own

For successful uploads you get JSON data in reply body with container and
object ID, like this:
```
{
        "object_id": "9ANhbry2ryjJY1NZbcjryJMRXG5uGNKd73kD3V1sVFsX",
        "container_id": "Dxhf4PNprrJHWWTG5RGLdfLkJiSQ3AQqit1MSnEPRkDZ"
}
```

### Metrics and Pprof

If enabled, Prometheus metrics are available at `/metrics/` path and Pprof at
`/debug/pprof`.
