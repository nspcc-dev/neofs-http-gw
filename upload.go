package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"mime/multipart"
	"os"
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
		tmp     *os.File
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

	if tmp, err = ioutil.TempFile("", "http-gate-upload-*"); err != nil {
		log.Error("could not prepare temporary file", zap.Error(err))
		c.Error("could not prepare temporary file", fasthttp.StatusBadRequest)
		return
	}

	defer func() {
		tmpName := tmp.Name()

		log.Debug("close temporary file", zap.Error(tmp.Close()))
		log.Debug("remove temporary file", zap.Error(os.RemoveAll(tmpName)))
	}()

	if form, err = c.MultipartForm(); err != nil {
		log.Error("could not receive multipart form", zap.Error(err))
		c.Error("could not receive multipart form: "+err.Error(), fasthttp.StatusBadRequest)

		return
	} else if ln := len(form.File); ln != 1 {
		log.Error("received multipart form with more then one file", zap.Int("count", ln))
		c.Error("received multipart form with more then one file", fasthttp.StatusBadRequest)

		return
	}

	for _, file := range form.File {
		if ln := len(file); ln != 1 {
			log.Error("received multipart form file should contains one record", zap.Int("count", ln))
			c.Error("received multipart form file should contains one record", fasthttp.StatusBadRequest)

			return
		}

		name = file[0].Filename

		if err = fasthttp.SaveMultipartFile(file[0], tmp.Name()); err != nil {
			log.Error("could not store uploaded file into temporary", zap.Error(err))
			c.Error("could not store uploaded file into temporary", fasthttp.StatusBadRequest)

			return
		}
	}

	filtered := a.hdr.Filter(&c.Request.Header)
	attributes := make([]*object.Attribute, 0, len(filtered))

	for key, val := range filtered {
		attribute := object.NewAttribute()
		attribute.SetKey(key)
		attribute.SetValue(val)

		attributes = append(attributes, attribute)
	}

	// Attribute FileName wasn't set from header
	if _, ok := filtered[object.AttributeFileName]; ok {
		filename := object.NewAttribute()
		filename.SetKey(object.AttributeFileName)
		filename.SetValue(name)

		attributes = append(attributes, filename)
	}

	// Attribute Timestamp wasn't set from header
	if _, ok := filtered[object.AttributeTimestamp]; ok {
		timestamp := object.NewAttribute()
		timestamp.SetKey(object.AttributeTimestamp)
		timestamp.SetValue(strconv.FormatInt(time.Now().Unix(), 10))

		attributes = append(attributes, timestamp)
	}

	raw := object.NewRaw()
	raw.SetContainerID(cid)
	raw.SetOwnerID(a.cli.Owner()) // should be various: from sdk / BearerToken
	raw.SetAttributes(attributes...)

	if addr, err = a.cli.Object().Put(c, raw.Object(), sdk.WithPutReader(tmp)); err != nil {
		log.Error("could not store file in NeoFS", zap.Error(err))
		c.Error("could not store file in NeoFS", fasthttp.StatusBadRequest)

		return
	} else if err = newPutResponse(addr).Encode(c); err != nil {
		log.Error("could not prepare response", zap.Error(err))
		c.Error("could not prepare response", fasthttp.StatusBadRequest)

		return
	}

	c.Response.SetStatusCode(fasthttp.StatusOK)
}
