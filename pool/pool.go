package pool

import (
	"context"

	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"google.golang.org/grpc"
)

type Client interface {
	// receive status of connection pool
	Status() error
	// worker should be run in goroutine to re-balancing
	Worker(context.Context)
	Connection(context.Context) (*grpc.ClientConn, error)
	Session(context.Context, *grpc.ClientConn) (*token.SessionToken, error)
}

type pool struct{}

func (p *pool) Status() error {
	return nil
}

func (p *pool) Worker(ctx context.Context) {
	panic("not implemented")
}

func (p *pool) Connection(ctx context.Context) (*grpc.ClientConn, error) {
	panic("not implemented")
}

func (p *pool) Session(ctx context.Context, conn *grpc.ClientConn) (*token.SessionToken, error) {
	panic("not implemented")
}

func New() Client {
	return &pool{}
}
