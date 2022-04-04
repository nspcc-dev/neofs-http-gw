package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nspcc-dev/neofs-api-go/v2/refs"
	sessionv2 "github.com/nspcc-dev/neofs-api-go/v2/session"
	"github.com/nspcc-dev/neofs-http-gw/internal/util"
	"github.com/nspcc-dev/neofs-http-gw/rest/v1/model"
	"github.com/nspcc-dev/neofs-http-gw/rest/v1/spec"
	"github.com/nspcc-dev/neofs-sdk-go/acl"
	"github.com/nspcc-dev/neofs-sdk-go/container"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/policy"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/nspcc-dev/neofs-sdk-go/session"
	"github.com/valyala/fasthttp"
)

const (
	defaultPlacementPolicy = "REP 3"
	defaultBasicAcl        = acl.PrivateBasicName
)

// PutContainers handler that creates container in NeoFS.
func (a *API) PutContainers(c *fasthttp.RequestCtx, params spec.PutContainersParams) {
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	stoken, err := prepareSessionToken(&c.Request.Header)
	if err != nil {
		a.logAndSendError(c, "prepare bearer token", err, fasthttp.StatusBadRequest)
		return
	}

	userAttributes := prepareUserAttributes(&c.Request.Header)

	var request model.ContainersPutRequest
	if err = json.NewDecoder(c.RequestBodyStream()).Decode(&request); err != nil {
		a.logAndSendError(c, "couldn't decode container put request", err, fasthttp.StatusBadRequest)
		return
	}

	cnrID, err := createContainer(ctx, a.pool, stoken, &request, userAttributes)
	if err != nil {
		a.logAndSendError(c, "couldn't create container", err, fasthttp.StatusBadRequest)
		return
	}

	resp := &model.ContainersPutResponse{
		ContainerID: cnrID.String(),
	}

	a.encodeAndSend(c, resp)
}

// GetContainersContainerId handler that returns container info.
func (a *API) GetContainersContainerId(c *fasthttp.RequestCtx, containerId string) {
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	//containerId, _ := c.UserValue("containerId").(string)
	cnr, err := getContainer(ctx, a.pool, containerId)
	if err != nil {
		a.logAndSendError(c, "could not get container", err, fasthttp.StatusBadRequest)
		return
	}

	attrs := make([]model.Attribute, len(cnr.Attributes()))
	for i, attr := range cnr.Attributes() {
		attrs[i] = model.Attribute{Key: attr.Key(), Value: attr.Value()}
	}

	resp := &model.ContainerInfo{
		ContainerID:     containerId,
		Version:         cnr.Version().String(),
		OwnerID:         cnr.OwnerID().String(),
		BasicACL:        acl.BasicACL(cnr.BasicACL()).String(),
		PlacementPolicy: strings.Join(policy.Encode(cnr.PlacementPolicy()), " "),
		Attributes:      attrs,
	}

	a.encodeAndSend(c, resp)
}

func prepareUserAttributes(header *fasthttp.RequestHeader) map[string]string {
	filtered := util.FilterHeaders(header)
	delete(filtered, container.AttributeName)
	delete(filtered, container.AttributeTimestamp)
	return filtered
}

func getContainer(ctx context.Context, p *pool.Pool, containerId string) (*container.Container, error) {
	var cnrId cid.ID
	if err := cnrId.Parse(containerId); err != nil {
		return nil, fmt.Errorf("parse container id '%s': %w", containerId, err)
	}

	return p.GetContainer(ctx, &cnrId)
}

func createContainer(ctx context.Context, p *pool.Pool, stoken *session.Token, request *model.ContainersPutRequest, userAttrs map[string]string) (*cid.ID, error) {
	if request.PlacementPolicy == "" {
		request.PlacementPolicy = defaultPlacementPolicy
	}
	pp, err := policy.Parse(request.PlacementPolicy)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse placement policy: %w", err)
	}

	if request.BasicACL == "" {
		request.BasicACL = defaultBasicAcl
	}
	basicAcl, err := acl.ParseBasicACL(request.BasicACL)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse basic acl: %w", err)
	}

	cnrOptions := []container.Option{
		container.WithPolicy(pp),
		container.WithCustomBasicACL(basicAcl),
		container.WithAttribute(container.AttributeName, request.ContainerName),
		container.WithAttribute(container.AttributeTimestamp, strconv.FormatInt(time.Now().Unix(), 10)),
	}

	for key, val := range userAttrs {
		cnrOptions = append(cnrOptions, container.WithAttribute(key, val))
	}

	cnr := container.New(cnrOptions...)
	cnr.SetOwnerID(stoken.OwnerID())
	cnr.SetSessionToken(stoken)

	container.SetNativeName(cnr, request.ContainerName)

	cnrID, err := p.PutContainer(ctx, cnr)
	if err != nil {
		return nil, fmt.Errorf("could put object to neofs: %w", err)
	}

	prm := &pool.ContainerPollingParams{
		CreationTimeout: 20 * time.Second,
		PollInterval:    2 * time.Second,
	}

	if err = p.WaitForContainerPresence(ctx, cnrID, prm); err != nil {
		return nil, fmt.Errorf("wait for container presence: %w", err)
	}

	return cnrID, nil
}

func prepareSessionToken(header *fasthttp.RequestHeader) (*session.Token, error) {
	stoken, err := fetchSessionToken(header)
	if err != nil {
		return nil, fmt.Errorf("could not fetch session token: %w", err)
	}

	signBase64 := header.Peek(XNeofsTokenSignature)
	if signBase64 == nil {
		return nil, fmt.Errorf("missing header %s", XNeofsTokenSignature)
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
	stoken.ToV2().SetSignature(v2signature)

	if !stoken.VerifySignature() {
		err = fmt.Errorf("invalid signature")
	}

	return stoken, err
}

func fetchSessionToken(header *fasthttp.RequestHeader) (*session.Token, error) {
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

	body := new(sessionv2.TokenBody)
	if err = body.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("can't unmarshal bearer token: %w", err)
	}

	tkn := new(session.Token)
	tkn.ToV2().SetBody(body)

	return tkn, nil
}
