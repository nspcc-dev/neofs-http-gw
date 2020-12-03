# How to check

1. Create container
```
→ neofs-cli -k /path/to/user.key -r s01.neofs.devenv:8080 container create --name TestStorage --basic-acl public -p "REP 1 IN X CBF 1 SELECT 1 FROM * AS X" --await
container ID: 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM
awaiting...
container has been persisted on sidechain
```

2. Put object into container

```
→ neofs-cli -k /path/to/user.key -r s01.neofs.devenv:8080 object put --cid 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM --file /path/to/1.jpeg
[/path/to/1.jpeg] Object successfully stored
  ID: GTUokhLMtEq1Kh1nzSVCsWybFvVHFQyhZHaVXZBrYd3W
  CID: 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM
```

3. Check that object can be fetched by oldest API

```
→ curl -sSI -XGET http://http.neofs.devenv/get/88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM/GTUokhLMtEq1Kh1nzSVCsWybFvVHFQyhZHaVXZBrYd3W
HTTP/1.1 200 OK
Date: Thu, 03 Dec 2020 15:04:52 GMT
Content-Type: image/jpeg
Content-Length: 93077
x-object-id: GTUokhLMtEq1Kh1nzSVCsWybFvVHFQyhZHaVXZBrYd3W
x-owner-id: NTrezR3C4X8aMLVg7vozt5wguyNfFhwuFx
x-container-id: 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM
x-FileName: 1.jpeg
x-Timestamp: 1607006318
Last-Modified: Thu, 03 Dec 2020 17:38:38 MSK
Content-Disposition: inline; filename=1.jpeg
```

4. Check that object can be fetched by newest API

```
→ curl -sSI -XGET http://http.neofs.devenv/get_by_attribute/88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM/FileName/1.jpeg
HTTP/1.1 200 OK
Date: Thu, 03 Dec 2020 15:04:52 GMT
Content-Type: image/jpeg
Content-Length: 93077
x-object-id: GTUokhLMtEq1Kh1nzSVCsWybFvVHFQyhZHaVXZBrYd3W
x-owner-id: NTrezR3C4X8aMLVg7vozt5wguyNfFhwuFx
x-container-id: 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM
x-FileName: 1.jpeg
x-Timestamp: 1607006318
Last-Modified: Thu, 03 Dec 2020 17:38:38 MSK
Content-Disposition: inline; filename=1.jpeg
```

5. Put second object with same name

```
→ neofs-cli -k /path/to/user.key -r s01.neofs.devenv:8080 object put --cid 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM --file /path/to/1.jpeg
[/path/to/1.jpeg] Object successfully stored
  ID: 14Q3AhJhPyJzWrmiYMzswRDY4cXSUgKPSAEDxadkHKga
  CID: 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM

```

6. Check that object can be fetched by oldest API

```
→ curl -sSI -XGET http://http.neofs.devenv/get/88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM/14Q3AhJhPyJzWrmiYMzswRDY4cXSUgKPSAEDxadkHKga
HTTP/1.1 200 OK
Date: Thu, 03 Dec 2020 15:07:51 GMT
Content-Type: image/jpeg
Content-Length: 93077
x-object-id: 14Q3AhJhPyJzWrmiYMzswRDY4cXSUgKPSAEDxadkHKga
x-owner-id: NTrezR3C4X8aMLVg7vozt5wguyNfFhwuFx
x-container-id: 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM
x-FileName: 1.jpeg
x-Timestamp: 1607006355
Last-Modified: Thu, 03 Dec 2020 17:39:15 MSK
Content-Disposition: inline; filename=1.jpeg
```

7. Retry fetch object by newest API

```
→ curl -sSI -XGET http://http.neofs.devenv/get_by_attribute/88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM/FileName/1.jpeg
HTTP/1.1 200 OK
Date: Thu, 03 Dec 2020 15:04:28 GMT
Content-Type: image/jpeg
Content-Length: 93077
x-object-id: 14Q3AhJhPyJzWrmiYMzswRDY4cXSUgKPSAEDxadkHKga
x-owner-id: NTrezR3C4X8aMLVg7vozt5wguyNfFhwuFx
x-container-id: 88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM
x-FileName: 1.jpeg
x-Timestamp: 1607006355
Last-Modified: Thu, 03 Dec 2020 17:39:15 MSK
Content-Disposition: inline; filename=1.jpeg
```

**http-gate log when find multiple objects**
```
2020-12-03T18:04:28.617+0300    debug   neofs-gw/receive.go:191 find multiple objects   {"cid": "88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM", "attr_key": "FileName", "attr_val": "1.jpeg", "object_ids": ["14Q3AhJhPyJzWrmiYMzswRDY4cXSUgKPSAEDxadkHKga", "GTUokhLMtEq1Kh1nzSVCsWybFvVHFQyhZHaVXZBrYd3W"], "show_object_id": "14Q3AhJhPyJzWrmiYMzswRDY4cXSUgKPSAEDxadkHKga"}
```

8. Check newest API when object not found

```
→ curl -sSI -XGET http://http.neofs.devenv/get_by_attribute/88GdaZFTcYJn1dqiSECss8kKPmmun6d6BfvC4zhwfLYM/FileName/2.jpeg
HTTP/1.1 404 Not Found
Date: Thu, 03 Dec 2020 15:11:07 GMT
Content-Type: text/plain; charset=utf-8
Content-Length: 9
```
