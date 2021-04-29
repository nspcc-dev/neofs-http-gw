package neofs

import (
	"context"
	"crypto/ecdsa"
	"io"

	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	"github.com/nspcc-dev/neofs-api-go/pkg/container"
	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	"github.com/nspcc-dev/neofs-api-go/pkg/owner"
	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	"github.com/nspcc-dev/neofs-http-gate/connections"
)

type BaseOptions struct {
	Client       client.Client
	SessionToken *token.SessionToken
	BearerToken  *token.BearerToken
}

type PutOptions struct {
	BaseOptions
	Attributes  []*object.Attribute
	ContainerID *container.ID
	OwnerID     *owner.ID
	Reader      io.Reader
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

func (cp *neofsClientPlant) Object() ObjectClient {
	return &neofsObjectClient{
		key:  cp.key,
		pool: cp.pool,
	}
}

func (cp *neofsClientPlant) OwnerID() *owner.ID {
	return cp.ownerID
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
	rawObject := object.NewRaw()
	rawObject.SetContainerID(options.ContainerID)
	rawObject.SetOwnerID(options.OwnerID)
	rawObject.SetAttributes(options.Attributes...)
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
