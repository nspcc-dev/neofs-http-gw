package resolver

import (
	"context"
	"fmt"

	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/ns"
)

const (
	NNSResolver = "nns"
	DNSResolver = "dns"
)

// NeoFS represents virtual connection to the NeoFS network.
type NeoFS interface {
	// SystemDNS reads system DNS network parameters of the NeoFS.
	//
	// Returns exactly on non-zero value. Returns any error encountered
	// which prevented the parameter to be read.
	SystemDNS(context.Context) (string, error)
}

type Config struct {
	NeoFS      NeoFS
	RPCAddress string
}

type ContainerResolver struct {
	Name    string
	resolve func(context.Context, string) (*cid.ID, error)

	next *ContainerResolver
}

func (r *ContainerResolver) SetResolveFunc(fn func(context.Context, string) (*cid.ID, error)) {
	r.resolve = fn
}

func (r *ContainerResolver) Resolve(ctx context.Context, name string) (*cid.ID, error) {
	cnrID, err := r.resolve(ctx, name)
	if err != nil {
		if r.next != nil {
			cnrID, inErr := r.next.Resolve(ctx, name)
			if inErr != nil {
				return nil, fmt.Errorf("%s; %w", err.Error(), inErr)
			}
			return cnrID, nil
		}
		return nil, err
	}
	return cnrID, nil
}

func NewResolver(order []string, cfg *Config) (*ContainerResolver, error) {
	if len(order) == 0 {
		return nil, fmt.Errorf("resolving order must not be empty")
	}

	bucketResolver, err := newResolver(order[len(order)-1], cfg, nil)
	if err != nil {
		return nil, err
	}

	for i := len(order) - 2; i >= 0; i-- {
		resolverName := order[i]
		next := bucketResolver

		bucketResolver, err = newResolver(resolverName, cfg, next)
		if err != nil {
			return nil, err
		}
	}

	return bucketResolver, nil
}

func newResolver(name string, cfg *Config, next *ContainerResolver) (*ContainerResolver, error) {
	switch name {
	case DNSResolver:
		return NewDNSResolver(cfg.NeoFS, next)
	case NNSResolver:
		return NewNNSResolver(cfg.RPCAddress, next)
	default:
		return nil, fmt.Errorf("unknown resolver: %s", name)
	}
}

func NewDNSResolver(neoFS NeoFS, next *ContainerResolver) (*ContainerResolver, error) {
	if neoFS == nil {
		return nil, fmt.Errorf("pool must not be nil for DNS resolver")
	}

	var dns ns.DNS

	resolveFunc := func(ctx context.Context, name string) (*cid.ID, error) {
		domain, err := neoFS.SystemDNS(ctx)
		if err != nil {
			return nil, fmt.Errorf("read system DNS parameter of the NeoFS: %w", err)
		}

		domain = name + "." + domain
		cnrID, err := dns.ResolveContainerName(domain)
		if err != nil {
			return nil, fmt.Errorf("couldn't resolve container '%s' as '%s': %w", name, domain, err)
		}
		return &cnrID, nil
	}

	return &ContainerResolver{
		Name: DNSResolver,

		resolve: resolveFunc,
		next:    next,
	}, nil
}

func NewNNSResolver(rpcAddress string, next *ContainerResolver) (*ContainerResolver, error) {
	var nns ns.NNS

	if err := nns.Dial(rpcAddress); err != nil {
		return nil, fmt.Errorf("could not dial nns: %w", err)
	}

	resolveFunc := func(_ context.Context, name string) (*cid.ID, error) {
		cnrID, err := nns.ResolveContainerName(name)
		if err != nil {
			return nil, fmt.Errorf("couldn't resolve container '%s': %w", name, err)
		}
		return &cnrID, nil
	}

	return &ContainerResolver{
		Name: NNSResolver,

		resolve: resolveFunc,
		next:    next,
	}, nil
}
