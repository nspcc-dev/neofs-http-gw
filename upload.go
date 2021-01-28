package main

import (
	"encoding/json"
	"io"
	"mime/multipart"
	"strconv"
	"time"

	sdk "github.com/nspcc-dev/cdn-sdk"
	"github.com/nspcc-dev/neofs-api-go/pkg/container"
	"github.com/nspcc-dev/neofs-api-go/pkg/object"
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

func (pr *putResponse) Encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(pr)
}

func (a *app) upload(c *fasthttp.RequestCtx) {
	var (
		err     error
		name    string
		tmp     io.Reader
		addr    *object.Address
		form    *multipart.Form
		cid     = container.NewID()
		sCID, _ = c.UserValue("cid").(string)

		log = a.log.With(zap.String("cid", sCID))
	)

	if err = cid.Parse(sCID); err != nil {
		log.Error("wrong container id", zap.Error(err))
		c.Error("wrong container id", fasthttp.StatusBadRequest)
		return
	}

	defer func() {
		// if temporary reader can be closed - close it
		if closer := tmp.(io.Closer); closer != nil {
			log.Debug("close temporary multipart/form file", zap.Error(closer.Close()))
		}

		if form == nil {
			return
		}

		log.Debug("cleanup multipart/form", zap.Error(form.RemoveAll()))
	}()

	// tries to receive multipart/form or throw error
	if form, err = c.MultipartForm(); err != nil {
		log.Error("could not receive multipart/form", zap.Error(err))
		c.Error("could not receive multipart/form: "+err.Error(), fasthttp.StatusBadRequest)

		return
	}

	// checks that received multipart/form contains only one `file` per request
	if ln := len(form.File); ln != 1 {
		log.Error("received multipart/form with more then one file", zap.Int("count", ln))
		c.Error("received multipart/form with more then one file", fasthttp.StatusBadRequest)

		return
	}

	for _, file := range form.File {
		// because multipart/form can contains multiple FileHeader records
		// we should check that we have only one per request or throw error
		if ln := len(file); ln != 1 {
			log.Error("received multipart/form file should contains one record", zap.Int("count", ln))
			c.Error("received multipart/form file should contains one record", fasthttp.StatusBadRequest)

			return
		}

		name = file[0].Filename

		// opens multipart/form file to work within or throw error
		if tmp, err = file[0].Open(); err != nil {
			log.Error("could not prepare uploaded file", zap.Error(err))
			c.Error("could not prepare uploaded file", fasthttp.StatusBadRequest)

			return
		}
	}

	filtered := a.hdr.Filter(&c.Request.Header)
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
		filename.SetValue(name)

		attributes = append(attributes, filename)
	}

	// sets Timestamp attribute if it wasn't set from header and enabled by settings
	if _, ok := filtered[object.AttributeTimestamp]; !ok && a.enableDefaultTimestamp {
		timestamp := object.NewAttribute()
		timestamp.SetKey(object.AttributeTimestamp)
		timestamp.SetValue(strconv.FormatInt(time.Now().Unix(), 10))

		attributes = append(attributes, timestamp)
	}

	// prepares new object and fill it
	raw := object.NewRaw()
	raw.SetContainerID(cid)
	raw.SetOwnerID(a.cli.Owner()) // should be various: from sdk / BearerToken
	raw.SetAttributes(attributes...)

	// tries to put file into NeoFS or throw error
	if addr, err = a.cli.Object().Put(c, raw.Object(), sdk.WithPutReader(tmp)); err != nil {
		log.Error("could not store file in NeoFS", zap.Error(err))
		c.Error("could not store file in NeoFS", fasthttp.StatusBadRequest)

		return
	}

	// tries to return response, otherwise, if something went wrong throw error
	if err = newPutResponse(addr).Encode(c); err != nil {
		log.Error("could not prepare response", zap.Error(err))
		c.Error("could not prepare response", fasthttp.StatusBadRequest)

		return
	}

	// reports status code and content type
	c.Response.SetStatusCode(fasthttp.StatusOK)
	c.Response.Header.SetContentType(jsonHeader)
}
