package main

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/nspcc-dev/neofs-api-go/pkg/container"
	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	"github.com/nspcc-dev/neofs-api-go/pkg/owner"
	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"github.com/nspcc-dev/neofs-http-gate/neofs"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type putResponse struct {
	OID string `json:"object_id"`
	CID string `json:"container_id"`
}

const jsonHeader = "application/json; charset=UTF-8"

func newPutResponse(addr *object.Address) *putResponse {
	return &putResponse{
		OID: addr.ObjectID().String(),
		CID: addr.ContainerID().String(),
	}
}

func (pr *putResponse) encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(pr)
}

func (a *app) fetchOwnerAndBearerToken(ctx context.Context) (*owner.ID, *token.BearerToken) {
	if token, err := loadBearerToken(ctx); err == nil && token != nil {
		return token.Issuer(), token
	}
	return a.plant.OwnerID(), nil
}

func (a *app) upload(c *fasthttp.RequestCtx) {
	var (
		err     error
		file    MultipartFile
		addr    *object.Address
		cid     = container.NewID()
		sCID, _ = c.UserValue("cid").(string)
		log     = a.log.With(zap.String("cid", sCID))
	)
	if err = storeBearerToken(c); err != nil {
		log.Error("could not fetch bearer token", zap.Error(err))
		c.Error("could not fetch bearer token", fasthttp.StatusBadRequest)
		return
	}
	if err = cid.Parse(sCID); err != nil {
		log.Error("wrong container id", zap.Error(err))
		c.Error("wrong container id", fasthttp.StatusBadRequest)
		return
	}
	defer func() {
		// if temporary reader can be closed - close it
		if file == nil {
			return
		}
		err := file.Close()
		log.Debug(
			"close temporary multipart/form file",
			zap.Stringer("address", addr),
			zap.String("filename", file.FileName()),
			zap.Error(err),
		)
	}()
	boundary := string(c.Request.Header.MultipartFormBoundary())
	if file, err = fetchMultipartFile(a.log, c.RequestBodyStream(), boundary); err != nil {
		log.Error("could not receive multipart/form", zap.Error(err))
		c.Error("could not receive multipart/form: "+err.Error(), fasthttp.StatusBadRequest)
		return
	}
	filtered := filterHeaders(a.log, &c.Request.Header)
	attributes := make([]*object.Attribute, 0, len(filtered))
	// prepares attributes from filtered headers
	for key, val := range filtered {
		attribute := object.NewAttribute()
		attribute.SetKey(key)
		attribute.SetValue(val)
		attributes = append(attributes, attribute)
	}
	// sets FileName attribute if it wasn't set from header
	if _, ok := filtered[object.AttributeFileName]; !ok {
		filename := object.NewAttribute()
		filename.SetKey(object.AttributeFileName)
		filename.SetValue(file.FileName())
		attributes = append(attributes, filename)
	}
	// sets Timestamp attribute if it wasn't set from header and enabled by settings
	if _, ok := filtered[object.AttributeTimestamp]; !ok && a.enableDefaultTimestamp {
		timestamp := object.NewAttribute()
		timestamp.SetKey(object.AttributeTimestamp)
		timestamp.SetValue(strconv.FormatInt(time.Now().Unix(), 10))
		attributes = append(attributes, timestamp)
	}
	oid, bt := a.fetchOwnerAndBearerToken(c)
	// prepares new object and fill it
	raw := object.NewRaw()
	raw.SetContainerID(cid)
	raw.SetOwnerID(oid)
	raw.SetAttributes(attributes...)
	// tries to put file into NeoFS or throw error
	// if addr, err = a.plant.Object().Put(c, raw.Object(), sdk.WithPutReader(file)); err != nil {
	// TODO: Take this from a sync pool.
	putOpts := new(neofs.PutOptions)
	putOpts.Client, putOpts.SessionToken, err = a.plant.GetReusableArtifacts(c)
	if err != nil {
		log.Error("failed to get neofs client's reusable artifacts", zap.Error(err))
		c.Error("failed to get neofs client's reusable artifacts", fasthttp.StatusInternalServerError)
		return
	}
	putOpts.BearerToken = bt
	putOpts.ContainerID = cid
	putOpts.OwnerID = oid
	putOpts.PrepareObjectOnsite = false
	putOpts.Reader = file
	if addr, err = a.plant.Object().Put(c, putOpts); err != nil {
		log.Error("could not store file in NeoFS", zap.Error(err))
		c.Error("could not store file in NeoFS", fasthttp.StatusBadRequest)
		return
	}
	// tries to return response, otherwise, if something went wrong throw error
	if err = newPutResponse(addr).encode(c); err != nil {
		log.Error("could not prepare response", zap.Error(err))
		c.Error("could not prepare response", fasthttp.StatusBadRequest)

		return
	}
	// reports status code and content type
	c.Response.SetStatusCode(fasthttp.StatusOK)
	c.Response.Header.SetContentType(jsonHeader)
}
