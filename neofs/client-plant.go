package neofs

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"io"

	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	"github.com/nspcc-dev/neofs-api-go/pkg/container"
	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	"github.com/nspcc-dev/neofs-api-go/pkg/owner"
	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"github.com/nspcc-dev/neofs-http-gate/connections"
	objectCore "github.com/nspcc-dev/neofs-node/pkg/core/object"
	"github.com/nspcc-dev/neofs-node/pkg/services/object_manager/transformer"
)

const maxObjectSize = uint64(1 << 28) // Limit objects to 256 MiB.

type BaseOptions struct {
	Client       client.Client
	SessionToken *token.SessionToken
	BearerToken  *token.BearerToken
}

type PutOptions struct {
	BaseOptions
	ContainerID         *container.ID
	OwnerID             *owner.ID
	PrepareObjectOnsite bool
	Reader              io.Reader
}

type GetOptions struct {
	BaseOptions
	ObjectAddress *object.Address
	Writer        io.Writer
}

type SearchOptions struct {
	BaseOptions
	ContainerID *container.ID
	Attribute   struct {
		Key   string
		Value string
	}
}

type DeleteOptions struct {
	BaseOptions
	ObjectAddress *object.Address
}

type ObjectClient interface {
	Put(context.Context, *PutOptions) (*object.Address, error)
	Get(context.Context, *GetOptions) (*object.Object, error)
	Search(context.Context, *SearchOptions) ([]*object.ID, error)
	Delete(context.Context, *DeleteOptions) error
}

type ClientPlant interface {
	ConnectionArtifacts() (client.Client, *token.SessionToken, error)
	Object() ObjectClient
	OwnerID() *owner.ID
}

type neofsObjectClient struct {
	key  *ecdsa.PrivateKey
	pool connections.Pool
}

type neofsClientPlant struct {
	key     *ecdsa.PrivateKey
	ownerID *owner.ID
	pool    connections.Pool
}

func (cp *neofsClientPlant) ConnectionArtifacts() (client.Client, *token.SessionToken, error) {
	return cp.pool.ConnectionArtifacts()
}

func (cc *neofsClientPlant) Object() ObjectClient {
	return &neofsObjectClient{
		key:  cc.key,
		pool: cc.pool,
	}
}

func (cc *neofsClientPlant) OwnerID() *owner.ID {
	return cc.ownerID
}

func NewClientPlant(ctx context.Context, pool connections.Pool, creds Credentials) (ClientPlant, error) {
	return &neofsClientPlant{key: creds.PrivateKey(), ownerID: creds.Owner(), pool: pool}, nil
}

func (oc *neofsObjectClient) Put(ctx context.Context, options *PutOptions) (*object.Address, error) {
	var (
		err      error
		objectID *object.ID
	)
	address := object.NewAddress()
	if options.PrepareObjectOnsite {
		rawObject := objectCore.NewRaw()
		rawObject.SetContainerID(options.ContainerID)
		rawObject.SetOwnerID(options.OwnerID)
		ns := newNetworkState(ctx, options.Client)
		objectTarget := transformer.NewPayloadSizeLimiter(maxObjectSize, func() transformer.ObjectTarget {
			return transformer.NewFormatTarget(&transformer.FormatterParams{
				Key: oc.key,
				NextTarget: &remoteClientTarget{
					ctx:    ctx,
					client: options.Client,
				},
				NetworkState: ns,
			})
		})
		if err = ns.LastError(); err != nil {
			return nil, err
		}
		err = objectTarget.WriteHeader(rawObject)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(objectTarget, options.Reader)
		if err != nil {
			return nil, err
		}
		var ids *transformer.AccessIdentifiers
		ids, err = objectTarget.Close()
		if err != nil {
			return nil, err
		}
		address.SetObjectID(ids.SelfID())
	} else {
		rawObject := object.NewRaw()
		rawObject.SetContainerID(options.ContainerID)
		rawObject.SetOwnerID(options.OwnerID)
		ops := new(client.PutObjectParams).
			WithObject(rawObject.Object()).
			WithPayloadReader(options.Reader)
		objectID, err = options.Client.PutObject(
			ctx,
			ops,
			client.WithSession(options.SessionToken),
			client.WithBearer(options.BearerToken),
		)
		if err != nil {
			return nil, err
		}
		address.SetObjectID(objectID)
	}
	address.SetContainerID(options.ContainerID)
	return address, nil
}

func (oc *neofsObjectClient) Get(ctx context.Context, options *GetOptions) (*object.Object, error) {
	var (
		err error
		obj *object.Object
	)
	ops := new(client.GetObjectParams).
		WithAddress(options.ObjectAddress).
		WithPayloadWriter(options.Writer)
	obj, err = options.Client.GetObject(
		ctx,
		ops,
		client.WithSession(options.SessionToken),
		client.WithBearer(options.BearerToken),
	)
	return obj, err
}

func (oc *neofsObjectClient) Search(ctx context.Context, options *SearchOptions) ([]*object.ID, error) {
	sfs := object.NewSearchFilters()
	sfs.AddRootFilter()
	sfs.AddFilter(options.Attribute.Key, options.Attribute.Value, object.MatchStringEqual)
	sops := new(client.SearchObjectParams)
	sops.WithContainerID(options.ContainerID)
	sops.WithSearchFilters(sfs)
	return options.Client.SearchObject(
		ctx,
		sops,
		client.WithSession(options.SessionToken),
		client.WithBearer(options.BearerToken),
	)
}

func (oc *neofsObjectClient) Delete(ctx context.Context, options *DeleteOptions) error {
	ops := new(client.DeleteObjectParams).WithAddress(options.ObjectAddress)
	err := options.Client.DeleteObject(
		ctx,
		ops,
		client.WithSession(options.SessionToken),
		client.WithBearer(options.BearerToken),
	)
	return err
}

type remoteClientTarget struct {
	ctx     context.Context
	client  client.Client
	object  *object.Object
	payload []byte
}

func (rct *remoteClientTarget) WriteHeader(raw *objectCore.RawObject) error {
	rct.object = raw.Object().SDK()
	return nil
}

func (rct *remoteClientTarget) Write(p []byte) (n int, err error) {
	rct.payload = append(rct.payload, p...)
	return len(p), nil
}

func (rct *remoteClientTarget) Close() (*transformer.AccessIdentifiers, error) {
	id, err := rct.client.PutObject(
		rct.ctx, new(client.PutObjectParams).
			WithObject(rct.object).
			WithPayloadReader(bytes.NewReader(rct.payload)),
	)
	if err != nil {
		return nil, err
	}
	return new(transformer.AccessIdentifiers).WithSelfID(id), nil
}

type networkState struct {
	ctx       context.Context
	client    client.Client
	lastError error
	onError   func(error)
}

func newNetworkState(ctx context.Context, client client.Client) *networkState {
	ns := &networkState{
		ctx:    ctx,
		client: client,
	}
	ns.onError = func(err error) { ns.lastError = err }
	return ns
}

func (ns *networkState) LastError() error {
	return ns.lastError
}

func (ns *networkState) CurrentEpoch() uint64 {
	ce, err := ns.client.NetworkInfo(ns.ctx)
	if err != nil {
		ns.onError(err)
	}
	return ce.CurrentEpoch()
}
