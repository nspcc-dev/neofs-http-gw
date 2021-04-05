package connections

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	"google.golang.org/grpc"
)

type PoolBuilderOptions struct {
	Key                     *ecdsa.PrivateKey
	NodeConnectionTimeout   time.Duration
	NodeRequestTimeout      time.Duration
	ClientRebalanceInterval time.Duration
}

type PoolBuilder struct {
	addresses []string
	weights   []float64
}

func (pb *PoolBuilder) AddNode(address string, weight float64) *PoolBuilder {
	pb.addresses = append(pb.addresses, address)
	pb.weights = append(pb.weights, weight)
	return pb
}

func (pb *PoolBuilder) Build(ctx context.Context, options *PoolBuilderOptions) (Pool, error) {
	totalWeight := 0.0
	for _, w := range pb.weights {
		totalWeight += w
	}
	if math.Abs(totalWeight-1.0) >= 1e-4 {
		return nil, errors.New("total weight must be equal to unity")
	}
	var cons = make([]*grpc.ClientConn, len(pb.addresses))
	for i, address := range pb.addresses {
		con, err := func() (*grpc.ClientConn, error) {
			toctx, c := context.WithTimeout(ctx, options.NodeConnectionTimeout)
			defer c()
			return grpc.DialContext(toctx, address, grpc.WithInsecure(), grpc.WithBlock())
		}()
		if err != nil {
			return nil, err
		}
		cons[i] = con
	}
	return new(pb.weights, options.Key, cons)
}

type Pool interface {
	Client() client.Client
}

type pool struct {
	generator *Generator
	clients   []client.Client
}

func new(weights []float64, key *ecdsa.PrivateKey, connections []*grpc.ClientConn) (Pool, error) {
	clients := make([]client.Client, len(weights))
	for i, con := range connections {
		c, err := client.New(client.WithDefaultPrivateKey(key), client.WithGRPCConnection(con))
		if err != nil {
			return nil, err
		}
		clients[i] = c
	}
	source := rand.NewSource(time.Now().UnixNano())
	return &pool{
		generator: NewGenerator(weights, source),
		clients:   clients,
	}, nil
}

func (p *pool) Client() client.Client {
	if len(p.clients) == 1 {
		return p.clients[0]
	}
	return p.clients[p.generator.Next()]
}
