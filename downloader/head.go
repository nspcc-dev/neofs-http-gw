package downloader

import (
	"net/http"
	"strconv"
	"time"

	"github.com/nspcc-dev/neofs-http-gw/response"
	"github.com/nspcc-dev/neofs-http-gw/tokens"
	"github.com/nspcc-dev/neofs-sdk-go/client"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

const sizeToDetectType = 512

func (r request) headObject(clnt pool.Object, objectAddress *object.Address) {
	var start = time.Now()
	if err := tokens.StoreBearerToken(r.RequestCtx); err != nil {
		r.log.Error("could not fetch and store bearer token", zap.Error(err))
		response.Error(r.RequestCtx, "could not fetch and store bearer token", fasthttp.StatusBadRequest)
		return
	}

	options := new(client.ObjectHeaderParams).WithAddress(objectAddress)
	bearerOpt := bearerOpts(r.RequestCtx)
	obj, err := clnt.GetObjectHeader(r.RequestCtx, options, bearerOpt)
	if err != nil {
		r.handleNeoFSErr(err, start)
		return
	}

	r.Response.Header.Set("Content-Length", strconv.FormatUint(obj.PayloadSize(), 10))
	var contentType string
	for _, attr := range obj.Attributes() {
		key := attr.Key()
		val := attr.Value()
		if !isValidToken(key) || !isValidValue(val) {
			continue
		}
		r.Response.Header.Set("X-Attribute-"+key, val)
		switch key {
		case object.AttributeTimestamp:
			value, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				r.log.Info("couldn't parse creation date",
					zap.String("key", key),
					zap.String("val", val),
					zap.Error(err))
				continue
			}
			r.Response.Header.Set("Last-Modified", time.Unix(value, 0).UTC().Format(http.TimeFormat))
		case object.AttributeContentType:
			contentType = val
		}
	}
	r.Response.Header.Set("x-object-id", obj.ID().String())
	r.Response.Header.Set("x-owner-id", obj.OwnerID().String())
	r.Response.Header.Set("x-container-id", obj.ContainerID().String())

	if len(contentType) == 0 {
		objRange := object.NewRange()
		objRange.SetOffset(0)
		if sizeToDetectType < obj.PayloadSize() {
			objRange.SetLength(sizeToDetectType)
		} else {
			objRange.SetLength(obj.PayloadSize())
		}
		ops := new(client.RangeDataParams).WithAddress(objectAddress).WithRange(objRange)
		data, err := clnt.ObjectPayloadRangeData(r.RequestCtx, ops, bearerOpt)
		if err != nil {
			r.handleNeoFSErr(err, start)
			return
		}
		contentType = http.DetectContentType(data)
	}
	r.SetContentType(contentType)
}

// HeadByAddress handles head requests using simple cid/oid format.
func (d *Downloader) HeadByAddress(c *fasthttp.RequestCtx) {
	d.byAddress(c, request.headObject)
}

// HeadByAttribute handles attribute-based head requests.
func (d *Downloader) HeadByAttribute(c *fasthttp.RequestCtx) {
	d.byAttribute(c, request.headObject)
}
