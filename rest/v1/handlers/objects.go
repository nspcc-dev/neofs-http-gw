package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/nspcc-dev/neofs-api-go/v2/acl"
	"github.com/nspcc-dev/neofs-api-go/v2/refs"
	"github.com/nspcc-dev/neofs-http-gw/internal/util"
	"github.com/nspcc-dev/neofs-http-gw/rest/v1/model"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/nspcc-dev/neofs-sdk-go/token"
	"github.com/valyala/fasthttp"
)

const (
	XNeofsBearerSignature = "X-Neofs-Bearer-Signature"
	XNeofsBearerOwnerKey  = "X-Neofs-Bearer-Owner-Key"
	XNeofsBearerLifetime  = "X-Neofs-Bearer-Lifetime"
)

func (a *API) ObjectsPut(c *fasthttp.RequestCtx) {
	btoken, err := prepareBearerToken(&c.Request.Header)
	if err != nil {
		a.logAndSendError(c, "prepare bearer token", err, fasthttp.StatusBadRequest)
		return
	}

	var request model.ObjectsPutRequest
	if err = json.NewDecoder(c.RequestBodyStream()).Decode(&request); err != nil {
		a.logAndSendError(c, "couldn't decode object put request", err, fasthttp.StatusBadRequest)
		return
	}

	var cnrID cid.ID
	if err = cnrID.Parse(request.ContainerID); err != nil {
		a.logAndSendError(c, "couldn't parse container id", err, fasthttp.StatusBadRequest)
		return
	}

	payload, err := base64.StdEncoding.DecodeString(request.Payload)
	if err != nil {
		a.logAndSendError(c, "couldn't decode payload", err, fasthttp.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithCancel(c)
	defer cancel()

	prm := util.PrmAttributes{
		DefaultTimestamp: a.defaultTimestamp,
		DefaultFileName:  request.FileName,
	}
	attributes, err := util.GetObjectAttributes(ctx, &c.Request.Header, a.pool, prm)
	if err != nil {
		a.logAndSendError(c, "could not get object attributes", err, fasthttp.StatusBadRequest)
		return
	}

	obj := object.New()
	obj.SetContainerID(&cnrID)
	obj.SetOwnerID(btoken.OwnerID())
	obj.SetPayload(payload)
	obj.SetAttributes(attributes...)

	objID, err := a.pool.PutObject(ctx, *obj, nil, pool.WithBearer(btoken))
	if err != nil {
		a.logAndSendError(c, "could put object to neofs", err, fasthttp.StatusBadRequest)
		return
	}

	resp := &model.ObjectsPutResponse{
		ContainerID: request.ContainerID,
		ObjectID:    objID.String(),
	}

	enc := json.NewEncoder(c)
	enc.SetIndent("", "\t")
	if err = enc.Encode(resp); err != nil {
		a.logAndSendError(c, "could not encode response", err, fasthttp.StatusBadRequest)
		return
	}
	c.Response.SetStatusCode(fasthttp.StatusOK)
	c.Response.Header.SetContentType("application/json")
}

func prepareBearerToken(header *fasthttp.RequestHeader) (*token.BearerToken, error) {
	btoken, err := fetchBearerToken(header)
	if err != nil {
		return nil, fmt.Errorf("could not fetch bearer token: %w", err)
	}

	signBase64 := header.Peek(XNeofsBearerSignature)
	if signBase64 == nil {
		return nil, fmt.Errorf("missing header %s", XNeofsBearerSignature)
	}

	signature, err := base64.StdEncoding.DecodeString(string(signBase64))
	if err != nil {
		return nil, fmt.Errorf("couldn't decode bearer signature: %w", err)
	}

	ownerKey, err := fetchBearerOwner(header)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch bearer token owner key: %w", err)
	}

	v2signature := new(refs.Signature)
	v2signature.SetScheme(refs.ECDSA_SHA512)
	v2signature.SetSign(signature)
	v2signature.SetKey(ownerKey.Bytes())
	btoken.ToV2().SetSignature(v2signature)

	return btoken, btoken.VerifySignature()
}

func fetchBearerToken(header *fasthttp.RequestHeader) (*token.BearerToken, error) {
	auth := header.Peek(fasthttp.HeaderAuthorization)
	prefix := []byte("Bearer ")
	if auth == nil || !bytes.HasPrefix(auth, prefix) {
		return nil, fmt.Errorf("has not bearer token")
	}
	if auth = bytes.TrimPrefix(auth, prefix); len(auth) == 0 {
		return nil, fmt.Errorf("bearer token is empty")
	}

	data, err := base64.StdEncoding.DecodeString(string(auth))
	if err != nil {
		return nil, fmt.Errorf("can't base64-decode bearer token: %w", err)
	}

	body := new(acl.BearerTokenBody)
	if err = body.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("can't unmarshal bearer token: %w", err)
	}

	tkn := new(token.BearerToken)
	tkn.ToV2().SetBody(body)

	return tkn, nil
}
