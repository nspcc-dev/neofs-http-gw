package main

import (
	"context"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/nspcc-dev/neofs-api-go/object"
	"github.com/nspcc-dev/neofs-api-go/refs"
	"github.com/nspcc-dev/neofs-api-go/service"
	"github.com/pkg/errors"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (a *app) receiveFile(c *fasthttp.RequestCtx) {
	var (
		err     error
		cid     refs.CID
		oid     refs.ObjectID
		start   = time.Now()
		con     *grpc.ClientConn
		cli     object.Service_GetClient
		ctx     context.Context
		sCID, _ = c.UserValue("cid").(string)
		sOID, _ = c.UserValue("oid").(string)
	)

	log := a.log.With(
		// zap.String("node", con.Target()),
		zap.String("cid", sCID),
		zap.String("oid", sOID))

	if err = cid.Parse(sCID); err != nil {
		log.Error("wrong container id", zap.Error(err))

		c.Error("wrong container id", fasthttp.StatusBadRequest)
		return
	} else if err = oid.Parse(sOID); err != nil {
		log.Error("wrong object id", zap.Error(err))

		c.Error("wrong object id", fasthttp.StatusBadRequest)
		return
	}

	{ // try to connect or throw http error:
		ctx, cancel := context.WithTimeout(c, a.reqTimeout)
		defer cancel()

		if con, err = a.pool.getConnection(ctx); err != nil {
			log.Error("getConnection timeout", zap.Error(err))
			c.Error("could not get alive connection", fasthttp.StatusBadRequest)
			return
		}
	}

	ctx, cancel := context.WithTimeout(c, a.reqTimeout)
	defer cancel()

	log = log.With(zap.String("node", con.Target()))

	defer func() {
		if err != nil {
			return
		}

		log.Info("object sent to client", zap.Stringer("elapsed", time.Since(start)))
	}()

	req := &object.GetRequest{Address: refs.Address{ObjectID: oid, CID: cid}}
	req.SetTTL(service.SingleForwardingTTL)

	if err = service.SignDataWithSessionToken(a.key, req); err != nil {
		log.Error("could not sign request", zap.Error(err))
		c.Error("could not sign request", fasthttp.StatusBadRequest)
		return
	}

	if cli, err = object.NewServiceClient(con).Get(ctx, req); err != nil {
		log.Error("could not prepare connection", zap.Error(err))

		c.Error("could not prepare connection", fasthttp.StatusBadRequest)
		return
	} else if err = receiveObject(c, cli); err != nil {
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
}

func receiveObject(c *fasthttp.RequestCtx, cli object.Service_GetClient) error {
	var (
		typ string
		content_disp_type string
		put = c.Request.URI().QueryArgs().GetBool("download")
	)

	for {
		resp, err := cli.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		switch o := resp.R.(type) {
		case *object.GetResponse_Object:
			obj := o.Object

			c.Response.Header.Set("Content-Length", strconv.FormatUint(obj.SystemHeader.PayloadLength, 10))
			c.Response.Header.Set("x-object-id", obj.SystemHeader.ID.String())
			c.Response.Header.Set("x-owner-id", obj.SystemHeader.OwnerID.String())
			c.Response.Header.Set("x-container-id", obj.SystemHeader.CID.String())

			for i := range obj.Headers {
				if hdr := obj.Headers[i].GetUserHeader(); hdr != nil {
					c.Response.Header.Set("x-"+hdr.Key, hdr.Value)

					if hdr.Key == object.FilenameHeader {
						if put { 
							content_disp_type = "attachment" 
						} else { content_disp_type = "inline"}
						// NOTE: we use path.Base because hdr.Value can be something like `/path/to/filename.ext`
						c.Response.Header.Set("Content-Disposition", content_disp_type+"; filename="+path.Base(hdr.Value))
					}
				}
			}

			if len(obj.Payload) > 0 {
				typ = http.DetectContentType(obj.Payload)
			}

			if _, err = c.Write(obj.Payload); err != nil {
				return err
			}

		case *object.GetResponse_Chunk:
			if typ == "" {
				typ = http.DetectContentType(o.Chunk)
			}

			if _, err = c.Write(o.Chunk); err != nil {
				return err
			}
		}
	}

	c.SetContentType(typ)

	return nil
}
