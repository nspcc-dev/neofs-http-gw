package neofs

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"io"
	"math"
	"time"

	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	"github.com/nspcc-dev/neofs-api-go/pkg/container"
	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	"github.com/nspcc-dev/neofs-api-go/pkg/owner"
	"github.com/nspcc-dev/neofs-api-go/pkg/token"
	objectCore "github.com/nspcc-dev/neofs-node/pkg/core/object"
	"github.com/nspcc-dev/neofs-node/pkg/services/object_manager/transformer"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

const (
	nodeConnectionTimeout = 10 * time.Second
	maxObjectSize         = uint64(1 << 26) // 64MiB
)

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
	GetReusableArtifacts(ctx context.Context) (client.Client, *token.SessionToken, error)
	Object() ObjectClient
	OwnerID() *owner.ID
}

type objectClient struct {
	key  *ecdsa.PrivateKey
	conn *grpc.ClientConn
}

type neofsClient struct {
	key     *ecdsa.PrivateKey
	ownerID *owner.ID
	conn    *grpc.ClientConn
}

func (cc *neofsClient) GetReusableArtifacts(ctx context.Context) (client.Client, *token.SessionToken, error) {
	c, err := client.New(client.WithDefaultPrivateKey(cc.key), client.WithGRPCConnection(cc.conn))
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create reusable neofs client")
	}
	st, err := c.CreateSession(ctx, math.MaxUint64)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create reusable neofs session token")
	}
	return c, st, nil
}

func (cc *neofsClient) Object() ObjectClient {
	return &objectClient{key: cc.key, conn: cc.conn}
}

func (cc *neofsClient) OwnerID() *owner.ID {
	return cc.ownerID
}

type Connection struct {
	address string
	weight  float64
}

type ConnectionList []Connection

func (p ConnectionList) Len() int           { return len(p) }
func (p ConnectionList) Less(i, j int) bool { return p[i].weight < p[j].weight }
func (p ConnectionList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (cl *ConnectionList) Add(address string, weight float64) ConnectionList {
	*cl = append(*cl, Connection{address, weight})
	return *cl
}

func NewClientPlant(ctx context.Context, connectionList ConnectionList, creds Credentials) (ClientPlant, error) {
	toctx, c := context.WithTimeout(ctx, nodeConnectionTimeout)
	defer c()
	// TODO: Use connection pool here.
	address := connectionList[0].address
	conn, err := grpc.DialContext(toctx, address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		if err == context.DeadlineExceeded {
			err = errors.New("failed to connect to neofs node")
		}
		return nil, err
	}
	return &neofsClient{
		key:     creds.PrivateKey(),
		ownerID: creds.Owner(),
		conn:    conn,
	}, nil
}

func (oc *objectClient) Put(ctx context.Context, options *PutOptions) (*object.Address, error) {
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

func (oc *objectClient) Get(ctx context.Context, options *GetOptions) (*object.Object, error) {
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

func (oc *objectClient) Search(ctx context.Context, options *SearchOptions) ([]*object.ID, error) {
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

func (oc *objectClient) Delete(ctx context.Context, options *DeleteOptions) error {
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
