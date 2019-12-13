package main

import (
	"context"
	"crypto/ecdsa"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/nspcc-dev/neofs-proto/object"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/keepalive"
)

type config struct {
	log     *zap.Logger
	timeout time.Duration
	key     *ecdsa.PrivateKey
	cli     object.ServiceClient
}

func main() {
	v := settings()

	log, err := newLogger(v)
	if err != nil {
		panic(err)
	}

	log.Info("running application", zap.String("version", v.GetString("app.version")))

	var (
		cfg   = new(config)
		grace = newGracefulContext(log)
	)

	if v.GetBool("verbose") {
		grpclog.SetLoggerV2(
			gRPCLogger(log))
	}

	cfg.log = log
	cfg.key = fetchKey(log, v)
	cfg.timeout = v.GetDuration("request_timeout")

	ctx, cancel := context.WithTimeout(grace, v.GetDuration("connect_timeout"))
	defer cancel()

	conn, err := grpc.DialContext(ctx, v.GetString("neofs_address"),
		grpc.WithBlock(),
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

	log.Info("stopping server", zap.Error(e.Shutdown(ctx)))
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
				log.Error("could not establish connection",
					zap.Stringer("state", state),
					zap.Any("connection", conn.Target()))
			}
		}
	}

	tick.Stop()
}
