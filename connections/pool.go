package connections

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	"google.golang.org/grpc"
)

type PoolBuilderOptions struct {
	Key                     *ecdsa.PrivateKey
	NodeConnectionTimeout   time.Duration
	NodeRequestTimeout      time.Duration
	ClientRebalanceInterval time.Duration
	weights                 []float64
	connections             []*grpc.ClientConn
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
	options.weights = pb.weights
	options.connections = cons
	return new(ctx, options)
}

type Pool interface {
	Client() client.Client
}

type pool struct {
	lock    sync.RWMutex
	sampler *Sampler
	clients []client.Client
	healthy []bool
}

func new(ctx context.Context, options *PoolBuilderOptions) (Pool, error) {
	n := len(options.weights)
	clients := make([]client.Client, n)
	healthy := make([]bool, n)
	for i, con := range options.connections {
		c, err := client.New(client.WithDefaultPrivateKey(options.Key), client.WithGRPCConnection(con))
		if err != nil {
			return nil, err
		}
		clients[i] = c
		healthy[i] = true
	}
	source := rand.NewSource(time.Now().UnixNano())
	pool := &pool{
		sampler: NewSampler(options.weights, source),
		clients: clients,
		healthy: healthy,
	}
	go func() {
		ticker := time.NewTimer(options.ClientRebalanceInterval)
		for range ticker.C {
			ok := true
			for i, client := range pool.clients {
				func() {
					tctx, c := context.WithTimeout(ctx, options.NodeRequestTimeout)
					defer c()
					if _, err := client.EndpointInfo(tctx); err != nil {
						ok = false
					}
					pool.lock.Lock()
					pool.healthy[i] = ok
					pool.lock.Unlock()
				}()
			}
			ticker.Reset(options.ClientRebalanceInterval)
		}
	}()
	return pool, nil
}

func (p *pool) Client() client.Client {
	p.lock.RLock()
	defer p.lock.RUnlock()
	if len(p.clients) == 1 {
		if p.healthy[0] {
			return p.clients[0]
		}
		return nil
	}
	var i *int = nil
	for k := 0; k < 10; k++ {
		i_ := p.sampler.Next()
		if p.healthy[i_] {
			i = &i_
		}
	}
	if i != nil {
		return p.clients[*i]
	}
	return nil
}
