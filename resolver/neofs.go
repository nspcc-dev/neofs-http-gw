package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/nspcc-dev/neofs-sdk-go/netmap"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
)

// NeoFSResolver represents virtual connection to the NeoFS network.
// It implements resolver.NeoFS.
type NeoFSResolver struct {
	pool *pool.Pool
}

// NewNeoFSResolver creates new NeoFSResolver using provided pool.Pool.
func NewNeoFSResolver(p *pool.Pool) *NeoFSResolver {
	return &NeoFSResolver{pool: p}
}

// SystemDNS implements resolver.NeoFS interface method.
func (x *NeoFSResolver) SystemDNS(ctx context.Context) (string, error) {
	networkInfo, err := x.pool.NetworkInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("read network info via client: %w", err)
	}

	var domain string

	networkInfo.NetworkConfig().IterateParameters(func(parameter *netmap.NetworkParameter) bool {
		if string(parameter.Key()) == "SystemDNS" {
			domain = string(parameter.Value())
			return true
		}

		return false
	})

	if domain == "" {
		return "", errors.New("system DNS parameter not found or empty")
	}

	return domain, nil
}
