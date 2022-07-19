# NeoFS HTTP Gateway configuration file

This section contains detailed NeoFS HTTP Gateway configuration file description
including default config values and some tips to set up configurable values.

There are some custom types used for brevity:

* `duration` -- string consisting of a number and a suffix. Suffix examples include `s` (seconds), `m` (minutes), `ms` (
  milliseconds).

# Structure

| Section         | Description                                           |
|-----------------|-------------------------------------------------------|
| no section      | [General parameters](#general-section)                |
| `wallet`        | [Wallet configuration](#wallet-section)               |
| `peers`         | [Nodes configuration](#peers-section)                 |
| `logger`        | [Logger configuration](#logger-section)               |
| `web`           | [Web configuration](#web-section)                     |
| `upload-header` | [Upload header configuration](#upload-header-section) |
| `zip`           | [ZIP configuration](#zip-section)                     |

# General section

```yaml
listen_address: 0.0.0.0:8082
tls_certificate: /path/to/tls/cert 
tls_key: /path/to/tls/key 

rpc_endpoint: http://node4.neofs:40332
resolve_order:
  - nns
  - dns

metrics: false
pprof: false

connect_timeout: 5s 
request_timeout: 5s 
rebalance_timer: 30s 
```

| Parameter         | Type       | Default value  | Description                                                                        |
|-------------------|------------|----------------|------------------------------------------------------------------------------------|
| `listen_address`  | `string`   | `0.0.0.0:8082` | The address that the gateway is listening on.                                      |
| `tls_certificate` | `string`   |                | Path to the TLS certificate.                                                       |
| `tls_key`         | `string`   |                | Path to the TLS key.                                                               |
| `rpc_endpoint`    | `string`   |                | The address of the RPC host to which the gateway connects to resolve bucket names. |
| `resolve_order`   | `[]string` | `[nns, dns]`   | Order of bucket name resolvers to use.                                             |
| `metrics`         | `bool`     | `false`        | Flag to enable and expose the prometheus metrics.                                  |
| `pprof`           | `bool`     | `false`        | Flag to enable the profiler.                                                       |
| `connect_timeout` | `duration` | `10s`          | Timeout to connect to a node.                                                      |
| `request_timeout` | `duration` | `15s`          | Timeout to check node health during rebalance.                                     |
| `rebalance_timer` | `duration` | `60s`          | Interval to check node health.                                                     |

# `wallet` section

```yaml
wallet:
  path: /path/to/wallet.json 
  address: NfgHwwTi3wHAS8aFAN243C5vGbkYDpqLHP 
  passphrase: pwd
```

| Parameter    | Type     | Default value | Description                                                              |
|--------------|----------|---------------|--------------------------------------------------------------------------|
| `path`       | `string` |               | Path to the wallet.                                                      |
| `address`    | `string` |               | Account address to get from wallet. If omitted default one will be used. |
| `passphrase` | `string` |               | Passphrase to decrypt wallet.                                            |

# `peers` section

```yaml
# Nodes configuration
# This configuration makes the gateway use the first node (node1.neofs:8080)
# while it's healthy. Otherwise, gateway uses the second node (node2.neofs:8080)
# for 10% of requests and the third node (node3.neofs:8080) for 90% of requests.
# Until nodes with the same priority level are healthy
# nodes with other priority are not used.
# The lower the value, the higher the priority.
peers:
  0:
    address: node1.neofs:8080
    priority: 1
    weight: 1
  1:
    address: node2.neofs:8080
    priority: 2
    weight: 0.1
  2:
    address: node3.neofs:8080
    priority: 2
    weight: 0.9
```

| Parameter  | Type     | Default value | Description                                                                                                                                             |
|------------|----------|---------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| `address`  | `string` |               | Address of storage node.                                                                                                                                |
| `priority` | `int`    | `1`           | It allows to group nodes and don't switch group until all nodes with the same priority will be unhealthy. The lower the value, the higher the priority. |
| `weight`   | `float`  | `1`           | Weight of node in the group with the same priority. Distribute requests to nodes proportionally to these values.                                        |

# `logger` section

```yaml
logger:
  level: debug
```

| Parameter | Type     | Default value | Description                                                                                        |
|-----------|----------|---------------|----------------------------------------------------------------------------------------------------|
| `level`   | `string` | `debug`       | Logging level.<br/>Possible values:  `debug`, `info`, `warn`, `error`, `dpanic`, `panic`, `fatal`. |


# `web` section

```yaml
web:
  read_buffer_size: 4096
  write_buffer_size: 4096
  read_timeout: 10m
  write_timeout: 5m
  stream_request_body: true
  max_request_body_size: 4194304
```

| Parameter               | Type       | Default value | Description                                                                                                                                                                                              |
|-------------------------|------------|---------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `read_buffer_size`      | `int`      | `4096`        | Per-connection buffer size for requests' reading. This also limits the maximum header size.                                                                                                              |
| `write_buffer_size`     | `int`      | `4096`        | Per-connection buffer size for responses' writing.                                                                                                                                                       |
| `read_timeout`          | `duration` | `10m`         | The amount of time allowed to read the full request including body. The connection's read deadline is reset when the connection opens, or for keep-alive connections after the first byte has been read. |
| `write_timeout`         | `duration` | `5m`          | The maximum duration before timing out writes of the response. It is reset after the request handler has returned.                                                                                       |
| `stream_request_body`   | `bool`     | `true`        | Enables request body streaming, and calls the handler sooner when given body is larger than the current limit.                                                                                           |
| `max_request_body_size` | `int`      | `4194304`     | Maximum request body size. The server rejects requests with bodies exceeding this limit.                                                                                                                 |


# `upload-header` section

```yaml
upload_header:
  use_default_timestamp: false
```

| Parameter               | Type   | Default value | Description                                                 |
|-------------------------|--------|---------------|-------------------------------------------------------------|
| `use_default_timestamp` | `bool` | `false`       | Create timestamp for object if it isn't provided by header. |


# `zip` section

```yaml
zip:
  compression: false 
```

| Parameter     | Type   | Default value | Description                                                  |
|---------------|--------|---------------|--------------------------------------------------------------|
| `compression` | `bool` | `false`       | Enable zip compression when download files by common prefix. |

