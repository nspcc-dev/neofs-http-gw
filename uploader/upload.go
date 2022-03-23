package uploader

import (
	"context"
	"encoding/json"
	"io"

	"github.com/nspcc-dev/neofs-http-gw/internal/util"
	"github.com/nspcc-dev/neofs-http-gw/response"
	"github.com/nspcc-dev/neofs-http-gw/tokens"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/nspcc-dev/neofs-sdk-go/object/address"
	oid "github.com/nspcc-dev/neofs-sdk-go/object/id"
	"github.com/nspcc-dev/neofs-sdk-go/owner"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/nspcc-dev/neofs-sdk-go/token"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

const (
	jsonHeader   = "application/json; charset=UTF-8"
	drainBufSize = 4096
)

// Uploader is an upload request handler.
type Uploader struct {
	log                    *zap.Logger
	pool                   *pool.Pool
	enableDefaultTimestamp bool
}

// New creates a new Uploader using specified logger, connection pool and
// other options.
func New(log *zap.Logger, conns *pool.Pool, enableDefaultTimestamp bool) *Uploader {
	return &Uploader{log, conns, enableDefaultTimestamp}
}

// Upload handles multipart upload request.
func (u *Uploader) Upload(c *fasthttp.RequestCtx) {
	var (
		err        error
		file       MultipartFile
		idObj      *oid.ID
		addr       = address.NewAddress()
		idCnr      = cid.New()
		scid, _    = c.UserValue("cid").(string)
		log        = u.log.With(zap.String("cid", scid))
		bodyStream = c.RequestBodyStream()
		drainBuf   = make([]byte, drainBufSize)
	)
	if err = tokens.StoreBearerToken(c); err != nil {
		log.Error("could not fetch bearer token", zap.Error(err))
		response.Error(c, "could not fetch bearer token", fasthttp.StatusBadRequest)
		return
	}
	if err = idCnr.Parse(scid); err != nil {
		log.Error("wrong container id", zap.Error(err))
		response.Error(c, "wrong container id", fasthttp.StatusBadRequest)
		return
	}
	defer func() {
		// If the temporary reader can be closed - let's close it.
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
	if file, err = fetchMultipartFile(u.log, bodyStream, boundary); err != nil {
		log.Error("could not receive multipart/form", zap.Error(err))
		response.Error(c, "could not receive multipart/form: "+err.Error(), fasthttp.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithCancel(c)
	defer cancel()

	prm := util.PrmAttributes{
		DefaultTimestamp: u.enableDefaultTimestamp,
		DefaultFileName:  file.FileName(),
	}

	attributes, err := util.GetObjectAttributes(ctx, &c.Request.Header, u.pool, prm)
	if err != nil {
		log.Error("could not get object attributes", zap.Error(err))
		response.Error(c, "could not get object attributes", fasthttp.StatusBadRequest)
		return
	}

	id, bt := u.fetchOwnerAndBearerToken(c)

	obj := object.New()
	obj.SetContainerID(idCnr)
	obj.SetOwnerID(id)
	obj.SetAttributes(attributes...)

	if idObj, err = u.pool.PutObject(ctx, *obj, file, pool.WithBearer(bt)); err != nil {
		log.Error("could not store file in neofs", zap.Error(err))
		response.Error(c, "could not store file in neofs", fasthttp.StatusBadRequest)
		return
	}

	addr.SetObjectID(idObj)
	addr.SetContainerID(idCnr)

	// Try to return the response, otherwise, if something went wrong, throw an error.
	if err = newPutResponse(addr).encode(c); err != nil {
		log.Error("could not prepare response", zap.Error(err))
		response.Error(c, "could not prepare response", fasthttp.StatusBadRequest)

		return
	}
	// Multipart is multipart and thus can contain more than one part which
	// we ignore at the moment. Also, when dealing with chunked encoding
	// the last zero-length chunk might be left unread (because multipart
	// reader only cares about its boundary and doesn't look further) and
	// it will be (erroneously) interpreted as the start of the next
	// pipelined header. Thus we need to drain the body buffer.
	for {
		_, err = bodyStream.Read(drainBuf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}
	// Report status code and content type.
	c.Response.SetStatusCode(fasthttp.StatusOK)
	c.Response.Header.SetContentType(jsonHeader)
}

func (u *Uploader) fetchOwnerAndBearerToken(ctx context.Context) (*owner.ID, *token.BearerToken) {
	if tkn, err := tokens.LoadBearerToken(ctx); err == nil && tkn != nil {
		return tkn.Issuer(), tkn
	}
	return u.pool.OwnerID(), nil
}

type putResponse struct {
	ObjectID    string `json:"object_id"`
	ContainerID string `json:"container_id"`
}

func newPutResponse(addr *address.Address) *putResponse {
	return &putResponse{
		ObjectID:    addr.ObjectID().String(),
		ContainerID: addr.ContainerID().String(),
	}
}

func (pr *putResponse) encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(pr)
}
