package main

import (
	"context"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/nspcc-dev/neofs-api/container"
	"github.com/nspcc-dev/neofs-api/object"
	"github.com/nspcc-dev/neofs-api/refs"
	"github.com/nspcc-dev/neofs-api/service"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func (r *router) receiveFile(c echo.Context) error {
	var (
		err      error
		cid      refs.CID
		oid      refs.ObjectID
		obj      *object.Object
		start    = time.Now()
		con      *grpc.ClientConn
		ctx      = c.Request().Context()
		cli      object.Service_GetClient
		download = c.QueryParam("download") != ""
	)

	log := r.log.With(
		// zap.String("node", con.Target()),
		zap.String("cid", c.Param("cid")),
		zap.String("oid", c.Param("oid")))

	if err = cid.Parse(c.Param("cid")); err != nil {
		log.Error("wrong container id", zap.Error(err))

		return echo.NewHTTPError(
			http.StatusBadRequest,
			errors.Wrap(err, "wrong container id").Error(),
		)
	} else if err = oid.Parse(c.Param("oid")); err != nil {
		log.Error("wrong object id", zap.Error(err))

		return echo.NewHTTPError(
			http.StatusBadRequest,
			errors.Wrap(err, "wrong object id").Error(),
		)
	}

	{ // try to connect or throw http error:
		ctx, cancel := context.WithTimeout(ctx, r.timeout)
		defer cancel()

		if con, err = r.pool.getConnection(ctx); err != nil {
			log.Error("getConnection timeout", zap.Error(err))
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	log = log.With(zap.String("node", con.Target()))

	defer func() {
		if err != nil {
			return
		}

		log.Error("object sent to client", zap.Stringer("elapsed", time.Since(start)))
	}()

	req := &object.GetRequest{Address: refs.Address{ObjectID: oid, CID: cid}}
	req.SetTTL(service.SingleForwardingTTL)

	if err = service.SignRequestHeader(r.key, req); err != nil {
		log.Error("could not sign request", zap.Error(err))
		return echo.NewHTTPError(
			http.StatusBadRequest,
			errors.Wrap(err, "could not sign request").Error())
	}

	if cli, err = object.NewServiceClient(con).Get(ctx, req); err != nil {
		log.Error("could not prepare connection", zap.Error(err))

		return echo.NewHTTPError(
			http.StatusBadRequest,
			errors.Wrap(err, "could not prepare connection").Error(),
		)
	} else if obj, err = receiveObject(cli); err != nil {
		log.Error("could not receive object",
			zap.Stringer("elapsed", time.Since(start)),
			zap.Error(err))

		switch {
		case strings.Contains(err.Error(), object.ErrNotFound.Error()),
			strings.Contains(err.Error(), container.ErrNotFound.Error()):
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		default:
			return echo.NewHTTPError(
				http.StatusBadRequest,
				errors.Wrap(err, "could not receive object").Error(),
			)
		}
	}

	log.Info("object fetched successfully")

	c.Response().Header().Set("Content-Length", strconv.FormatUint(obj.SystemHeader.PayloadLength, 10))
	c.Response().Header().Set("x-object-id", obj.SystemHeader.ID.String())
	c.Response().Header().Set("x-owner-id", obj.SystemHeader.OwnerID.String())
	c.Response().Header().Set("x-container-id", obj.SystemHeader.CID.String())

	for i := range obj.Headers {
		if hdr := obj.Headers[i].GetUserHeader(); hdr != nil {
			c.Response().Header().Set("x-"+hdr.Key, hdr.Value)

			if hdr.Key == object.FilenameHeader && download {
				// NOTE: we use path.Base because hdr.Value can be something like `/path/to/filename.ext`
				c.Response().Header().Set("Content-Disposition", "attachment; filename="+path.Base(hdr.Value))
			}
		}
	}

	return c.Blob(http.StatusOK,
		http.DetectContentType(obj.Payload),
		obj.Payload)
}

func receiveObject(cli object.Service_GetClient) (*object.Object, error) {
	var obj *object.Object
	for {
		resp, err := cli.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		} else if obj == nil {
			obj = resp.GetObject()
		}

		obj.Payload = append(obj.Payload, resp.GetChunk()...)
	}
	return obj, nil
}
