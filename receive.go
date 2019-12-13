package main

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/nspcc-dev/neofs-proto/container"
	"github.com/nspcc-dev/neofs-proto/object"
	"github.com/nspcc-dev/neofs-proto/refs"
	"github.com/nspcc-dev/neofs-proto/service"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func (cfg *config) receiveFile(c echo.Context) error {
	var (
		cid      refs.CID
		oid      refs.ObjectID
		obj      *object.Object
		download = c.QueryParam("download") != ""
	)

	cfg.log.Debug("try to fetch object from network",
		zap.String("cid", c.Param("cid")),
		zap.String("oid", c.Param("oid")))

	if err := cid.Parse(c.Param("cid")); err != nil {
		cfg.log.Error("wrong container id",
			zap.String("cid", c.Param("cid")),
			zap.String("oid", c.Param("oid")),
			zap.Error(err))

		return echo.NewHTTPError(
			http.StatusBadRequest,
			errors.Wrap(err, "wrong container id").Error(),
		)
	} else if err := oid.Parse(c.Param("oid")); err != nil {
		cfg.log.Error("wrong object id",
			zap.Stringer("cid", cid),
			zap.String("oid", c.Param("oid")),
			zap.Error(err))

		return echo.NewHTTPError(
			http.StatusBadRequest,
			errors.Wrap(err, "wrong object id").Error(),
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	req := &object.GetRequest{Address: refs.Address{ObjectID: oid, CID: cid}}
	req.SetTTL(service.SingleForwardingTTL)

	if err := service.SignRequestHeader(cfg.key, req); err != nil {
		cfg.log.Error("could not sign request",
			zap.Stringer("cid", cid),
			zap.Stringer("oid", oid),
			zap.Error(err))

		return echo.NewHTTPError(
			http.StatusBadRequest,
			errors.Wrap(err, "could not sign request").Error())
	}

	cli, err := cfg.cli.Get(ctx, req)
	if err != nil {
		cfg.log.Error("could not prepare connection",
			zap.Stringer("cid", cid),
			zap.Stringer("oid", oid),
			zap.Error(err))

		return echo.NewHTTPError(
			http.StatusBadRequest,
			errors.Wrap(err, "could not prepare connection").Error(),
		)
	} else if obj, err = receiveObject(cli); err != nil {
		cfg.log.Error("could not receive object",
			zap.Stringer("cid", cid),
			zap.Stringer("oid", oid),
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

	cfg.log.Info("object fetched successfully",
		zap.Stringer("cid", cid),
		zap.Stringer("oid", oid))

	c.Response().Header().Set("Content-Length", strconv.FormatUint(obj.SystemHeader.PayloadLength, 10))
	c.Response().Header().Set("x-object-id", obj.SystemHeader.ID.String())
	c.Response().Header().Set("x-owner-id", obj.SystemHeader.OwnerID.String())
	c.Response().Header().Set("x-container-id", obj.SystemHeader.CID.String())

	for i := range obj.Headers {
		if hdr := obj.Headers[i].GetUserHeader(); hdr != nil {
			c.Response().Header().Set("x-"+hdr.Key, hdr.Value)

			if hdr.Key == DropInFilenameHeader && download {
				c.Response().Header().Set("Content-Disposition", "attachment; filename="+hdr.Value)
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
