# REST API

NeoFS HTTP Gateway supports REST API. See [openapi spec](../rest/v1/spec/spec.yaml) for route details.

## Authentication

To interact with neofs-http-gw you have to use Bearer token authorization. To get bearer such bearer token you must:

1. Form bearer rules and send request to `/v1/auth`.
2. Sign the result response.
3. Use the following headers for subsequent requests to neofs-http-gw:
    * `Authorization` (the value must be `Bearer $response_from_step1}`)
    * `X-Neofs-Bearer-Signature` (the value must be base64 encoded signature from step 2)
    * `X-Neofs-Bearer-Owner-Key` (the value must be hex-encoded public key from pair that is used for signing in step 2)

**Note:** Response from step 1 is base64 encoded, but you have to sing binary data in step 2.

### Example

1. Forming bearer rules and send to `/v1/auth`

Sample bearer rules:

```json
{
  "records": [
    {
      "operation": "PUT",
      "action": "ALLOW",
      "filters": [],
      "targets": [
        {
          "role": "OTHERS",
          "keys": []
        }
      ]
    }
  ]
}
```

Sample response:

```text
CgwKABoICAMQASICCAMSGwoZNekWAqZbEQ+IBowBdPRqBnAFSoeI1h94GBoECGcYAw==
```

2. Sign the response from step 1 (first decode base64 response)

Signature sample (base64 encoded):

```text
BIGvEDmLerlYJNNTEDUBHGBFusgbt4j27bTaIZNiFzAjZySPfTAeVxZm1e6mehsyy5PTochXaWtaytiyeJuCxqI=
```

The public key from pair that is used to sing token:

```text
031a6c6fbbdf02ca351745fa86b9ba5a9452d785ac4f7fc2b7548ca2a46c4fcf4a
```

3. So now we can use the following header to make other requests:

* `Authorization: Bearer CgwKABoICAMQASICCAMSGwoZNekWAqZbEQ+IBowBdPRqBnAFSoeI1h94GBoECGcYAw==`
* `X-Neofs-Bearer-Signature: BIGvEDmLerlYJNNTEDUBHGBFusgbt4j27bTaIZNiFzAjZySPfTAeVxZm1e6mehsyy5PTochXaWtaytiyeJuCxqI=`
* `X-Neofs-Bearer-Owner-Key: 031a6c6fbbdf02ca351745fa86b9ba5a9452d785ac4f7fc2b7548ca2a46c4fcf4a`

## Put object

To put some object to NeoFS using REST API you should send `PUT` request to `/v1/objects`. You can use custom attributes
using headers `X-Attribute-*`

### Example

Suppose we want to put new object to NeoFS that has payload: `content of file` and
the following attributes:

* `FileName: myFile.txt`
* `Custom: some attribute value`

Sample headers:

```text
Authorization: Bearer CgwKABoICAMQASICCAMSGwoZNekWAqZbEQ+IBowBdPRqBnAFSoeI1h94GBoECGcYAw==
X-Neofs-Bearer-Signature: BIGvEDmLerlYJNNTEDUBHGBFusgbt4j27bTaIZNiFzAjZySPfTAeVxZm1e6mehsyy5PTochXaWtaytiyeJuCxqI=
X-Neofs-Bearer-Owner-Key: 031a6c6fbbdf02ca351745fa86b9ba5a9452d785ac4f7fc2b7548ca2a46c4fcf4a
X-Attribute-Custom: some attribute value
```

Sample body request (payload is base64 encoded):

```json
{
  "containerId": "5HZTn5qkRnmgSz9gSrw22CEdPPk6nQhkwf2Mgzyvkikv",
  "fileName": "myFile.txt",
  "payload": "Y29udGVudCBvZiBmaWxl"
}
```

Sample response:

```json
{
  "objectId": "8N3o7Dtr6T1xteCt6eRwhpmJ7JhME58Hyu1dvaswuTDd",
  "containerId": "5HZTn5qkRnmgSz9gSrw22CEdPPk6nQhkwf2Mgzyvkikv"
}
```
