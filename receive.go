package main

import (
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/nspcc-dev/cdn-neofs-sdk"
	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type detector struct {
	io.Writer
	sync.Once

	contentType string
}

func newDetector(w io.Writer) *detector {
	return &detector{Writer: w}
}

func (d *detector) Write(data []byte) (int, error) {
	d.Once.Do(func() {
		d.contentType = http.DetectContentType(data)
	})

	return d.Writer.Write(data)
}

func (a *app) receiveFile(c *fasthttp.RequestCtx) {
	var (
		err     error
		disp    = "inline"
		start   = time.Now()
		address = object.NewAddress()
		sCID, _ = c.UserValue("cid").(string)
		sOID, _ = c.UserValue("oid").(string)
		value   = strings.Join([]string{sCID, sOID}, "/")

		filename string
	)

	log := a.log.With(
		// zap.String("node", con.Target()),
		zap.String("cid", sCID),
		zap.String("oid", sOID))

	if err = address.Parse(value); err != nil {
		log.Error("wrong object address", zap.Error(err))
		c.Error("wrong object address", fasthttp.StatusBadRequest)
		return
	}

	writer := newDetector(c.Response.BodyWriter())
	obj, err := a.cli.Object().Get(c, address, sdk.WithGetWriter(writer))
	if err != nil {
		log.Error("could not receive object",
			zap.Stringer("elapsed", time.Since(start)),
			zap.Error(err))

		var (
			msg  = errors.Wrap(err, "could not receive object").Error()
			code = fasthttp.StatusBadRequest
		)

		if st, ok := status.FromError(errors.Cause(err)); ok && st != nil {
			if st.Code() == codes.NotFound {
				code = fasthttp.StatusNotFound
			}

			msg = st.Message()
		}

		c.Error(msg, code)
		return
	}

	if c.Request.URI().QueryArgs().GetBool("download") {
		disp = "attachment"
	}

	c.Response.Header.Set("Content-Length", strconv.FormatUint(obj.PayloadSize(), 10))
	c.Response.Header.Set("x-object-id", obj.ID().String())
	c.Response.Header.Set("x-owner-id", obj.OwnerID().String())
	c.Response.Header.Set("x-container-id", obj.ContainerID().String())

	for _, attr := range obj.Attributes() {
		key := attr.Key()
		val := attr.Value()

		c.Response.Header.Set("x-"+key, val)

		switch key {
		case object.AttributeFileName:
			filename = val
		case object.AttributeTimestamp:
			value, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				a.log.Info("couldn't parse creation date",
					zap.String("key", key),
					zap.String("val", val),
					zap.Error(err))
				continue
			}

			c.Response.Header.Set("Last-Modified",
				time.Unix(value, 0).Format(time.RFC1123))
		}

	}

	c.SetContentType(writer.contentType)
	c.Response.Header.Set("Content-Disposition", disp+"; filename="+path.Base(filename))
}
