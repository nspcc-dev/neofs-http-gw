package main

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/nspcc-dev/neofs-proto/object"
	"github.com/nspcc-dev/neofs-proto/refs"
	"github.com/nspcc-dev/neofs-proto/service"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
)

type config struct {
	log     *zap.Logger
	timeout time.Duration
	cli     object.ServiceClient
}

var defaultConfig = strings.NewReader(`
request_timeout: 5s
connect_timeout: 30s
listen_address: :8082
neofs_node_addr: :8080

logger:
  level: debug
  format: console
  trace_level: fatal
  no_disclaimer: true
  sampling:
    initial: 1000
    thereafter: 1000

keepalive:
  time: 100ms
  timeout: 10s
  permit_without_stream: true
`)

func main() {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix("GW")
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// set defaults:
	v.Set("app.name", "neofs-gw")
	v.Set("app.version", Version)

	if err := v.ReadConfig(defaultConfig); err != nil {
		panic(err)
	}

	log, err := newLogger(v)
	if err != nil {
		panic(err)
	}

	log.Info("running application", zap.String("version", v.GetString("app.version")))

	var (
		cfg   = new(config)
		grace = newGracefulContext(log)
	)

	cfg.log = log
	cfg.timeout = v.GetDuration("request_timeout")
	ctx, cancel := context.WithTimeout(grace, v.GetDuration("connect_timeout"))
	defer cancel()

	conn, err := grpc.DialContext(ctx, v.GetString("neofs_node_addr"),
		grpc.WithInsecure(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                v.GetDuration("keepalive.time"),
			Timeout:             v.GetDuration("keepalive.timeout"),
			PermitWithoutStream: v.GetBool("keepalive.permit_without_stream"),
		}),
	)
	if err != nil {
		log.Panic("could not connect to neofs-node",
			zap.String("neofs-node", v.GetString("neofs_node_addr")),
			zap.Error(err))
	}

	ctx, cancel = context.WithCancel(grace)
	defer cancel()

	go checkConnection(ctx, conn, log)
	cfg.cli = object.NewServiceClient(conn)

	e := echo.New()
	e.Debug = false
	e.HidePort = true
	e.HideBanner = true

	e.GET("/:cid/:oid", cfg.receiveFile)
	go func() {
		log.Info("run gateway server",
			zap.String("address", v.GetString("listen_address")))
		if err := e.Start(v.GetString("listen_address")); err != nil {
			log.Panic("could not start server", zap.Error(err))
		}
	}()

	<-ctx.Done()

	ctx, cancel = context.WithTimeout(context.TODO(), time.Second*30)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		log.Panic("could not stop server", zap.Error(err))
	}
}

func checkConnection(ctx context.Context, conn *grpc.ClientConn, log *zap.Logger) {
	tick := time.NewTicker(time.Second)
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-tick.C:
			switch state := conn.GetState(); state {
			case connectivity.Idle, connectivity.Connecting, connectivity.Ready:
				// It's ok..
			default:
				log.Panic("could not establish connection",
					zap.Stringer("state", state),
					zap.Any("connection", conn.Target()))
			}
		}
	}

	tick.Stop()
}

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

	cli, err := cfg.cli.Get(ctx, &object.GetRequest{
		Address: refs.Address{ObjectID: oid, CID: cid},
		TTL:     service.SingleForwardingTTL,
	})

	if err != nil {
		cfg.log.Error("could not prepare connection",
			zap.Stringer("cid", cid),
			zap.Stringer("oid", oid),
			zap.Error(err))

		return echo.NewHTTPError(
			// TODO: nginx doesn't return 500 errors from backend
			// http.StatusInternalServerError,
			http.StatusBadRequest,
			errors.Wrap(err, "could not prepare connection").Error(),
		)
	} else if obj, err = receiveObject(cli); err != nil {
		cfg.log.Error("could not receive object",
			zap.Stringer("cid", cid),
			zap.Stringer("oid", oid),
			zap.Error(err))

		if strings.Contains(err.Error(), object.ErrNotFound.Error()) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}

		return echo.NewHTTPError(
			// TODO: nginx doesn't return 500 errors from backend
			// http.StatusInternalServerError,
			http.StatusBadRequest,
			errors.Wrap(err, "could not receive object").Error(),
		)
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
