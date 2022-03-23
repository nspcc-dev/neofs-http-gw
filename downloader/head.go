package downloader

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nspcc-dev/neofs-http-gw/internal/util"
	"github.com/nspcc-dev/neofs-http-gw/response"
	"github.com/nspcc-dev/neofs-http-gw/tokens"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/nspcc-dev/neofs-sdk-go/object/address"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

// max bytes needed to detect content type according to http.DetectContentType docs.
const sizeToDetectType = 512

const (
	hdrObjectID    = "X-Object-Id"
	hdrOwnerID     = "X-Owner-Id"
	hdrContainerID = "X-Container-Id"
)

func (r request) headObject(clnt *pool.Pool, objectAddress *address.Address) {
	var start = time.Now()
	if err := tokens.StoreBearerToken(r.RequestCtx); err != nil {
		r.log.Error("could not fetch and store bearer token", zap.Error(err))
		response.Error(r.RequestCtx, "could not fetch and store bearer token", fasthttp.StatusBadRequest)
		return
	}

	bearerOpt := bearerOpts(r.RequestCtx)
	obj, err := clnt.HeadObject(r.RequestCtx, *objectAddress, bearerOpt)
	if err != nil {
		r.handleNeoFSErr(err, start)
		return
	}

	r.Response.Header.Set(fasthttp.HeaderContentLength, strconv.FormatUint(obj.PayloadSize(), 10))
	var contentType string
	for _, attr := range obj.Attributes() {
		key := attr.Key()
		val := attr.Value()
		if !isValidToken(key) || !isValidValue(val) {
			continue
		}
		r.Response.Header.Set(util.UserAttributeHeaderPrefix+key, val)
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
			r.Response.Header.Set(fasthttp.HeaderLastModified, time.Unix(value, 0).UTC().Format(http.TimeFormat))
		case object.AttributeContentType:
			contentType = val
		}
	}

	idsToResponse(&r.Response, obj)

	if len(contentType) == 0 {
		contentType, _, err = readContentType(obj.PayloadSize(), func(sz uint64) (io.Reader, error) {
			return clnt.ObjectRange(r.RequestCtx, *objectAddress, 0, sz, bearerOpt)
		})
		if err != nil && err != io.EOF {
			r.handleNeoFSErr(err, start)
			return
		}
	}
	r.SetContentType(contentType)
}

func idsToResponse(resp *fasthttp.Response, obj *object.Object) {
	resp.Header.Set(hdrObjectID, obj.ID().String())
	resp.Header.Set(hdrOwnerID, obj.OwnerID().String())
	resp.Header.Set(hdrContainerID, obj.ContainerID().String())
}

// HeadByAddress handles head requests using simple cid/oid format.
func (d *Downloader) HeadByAddress(c *fasthttp.RequestCtx) {
	d.byAddress(c, request.headObject)
}

// HeadByAttribute handles attribute-based head requests.
func (d *Downloader) HeadByAttribute(c *fasthttp.RequestCtx) {
	d.byAttribute(c, request.headObject)
}
