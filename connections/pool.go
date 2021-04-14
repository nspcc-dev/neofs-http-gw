package connections

import (
	"context"
	"crypto/ecdsa"
	"math/rand"
	"sync"
	"time"

	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type PoolBuilderOptions struct {
	Key                     *ecdsa.PrivateKey
	NodeConnectionTimeout   time.Duration
	NodeRequestTimeout      time.Duration
	ClientRebalanceInterval time.Duration
	SessionExpirationEpoch  uint64
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
	for i, w := range pb.weights {
		pb.weights[i] = w / totalWeight
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
	ConnectionArtifacts() (client.Client, *token.SessionToken, error)
}

type clientPack struct {
	client       client.Client
	sessionToken *token.SessionToken
	healthy      bool
}

type pool struct {
	lock        sync.RWMutex
	sampler     *Sampler
	clientPacks []*clientPack
}

func new(ctx context.Context, options *PoolBuilderOptions) (Pool, error) {
	clientPacks := make([]*clientPack, len(options.weights))
	for i, con := range options.connections {
		c, err := client.New(client.WithDefaultPrivateKey(options.Key), client.WithGRPCConnection(con))
		if err != nil {
			return nil, err
		}
		st, err := c.CreateSession(ctx, options.SessionExpirationEpoch)
		if err != nil {
			address := "unknown"
			if epi, err := c.EndpointInfo(ctx); err == nil {
				address = epi.NodeInfo().Address()
			}
			return nil, errors.Wrapf(err, "failed to create neofs session token for client %s", address)
		}
		clientPacks[i] = &clientPack{client: c, sessionToken: st, healthy: true}
	}
	source := rand.NewSource(time.Now().UnixNano())
	sampler := NewSampler(options.weights, source)
	pool := &pool{sampler: sampler, clientPacks: clientPacks}
	go func() {
		ticker := time.NewTimer(options.ClientRebalanceInterval)
		for range ticker.C {
			ok := true
			for i, clientPack := range pool.clientPacks {
				func() {
					tctx, c := context.WithTimeout(ctx, options.NodeRequestTimeout)
					defer c()
					if _, err := clientPack.client.EndpointInfo(tctx); err != nil {
						ok = false
					}
					pool.lock.Lock()
					pool.clientPacks[i].healthy = ok
					pool.lock.Unlock()
				}()
			}
			ticker.Reset(options.ClientRebalanceInterval)
		}
	}()
	return pool, nil
}

func (p *pool) ConnectionArtifacts() (client.Client, *token.SessionToken, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()
	if len(p.clientPacks) == 1 {
		cp := p.clientPacks[0]
		if cp.healthy {
			return cp.client, cp.sessionToken, nil
		}
		return nil, nil, errors.New("no healthy client")
	}
	attempts := 3 * len(p.clientPacks)
	for k := 0; k < attempts; k++ {
		i := p.sampler.Next()
		if cp := p.clientPacks[i]; cp.healthy {
			return cp.client, cp.sessionToken, nil
		}
	}
	return nil, nil, errors.New("no healthy client")
}
