package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/nspcc-dev/neofs-api-go/pkg/client"
	cid "github.com/nspcc-dev/neofs-api-go/pkg/container/id"
	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	"github.com/nspcc-dev/neofs-http-gw/tokens"
	"github.com/nspcc-dev/neofs-sdk-go/pkg/pool"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	detector struct {
		io.Reader
		err         error
		contentType string
		done        chan struct{}
		data        []byte
	}

	request struct {
		*fasthttp.RequestCtx
		log *zap.Logger
	}

	objectIDs []*object.ID

	errReader struct {
		data   []byte
		err    error
		offset int
	}
)

func newReader(data []byte, err error) *errReader {
	return &errReader{data: data, err: err}
}

func (r *errReader) Read(b []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(b, r.data[r.offset:])
	r.offset += n
	if r.offset >= len(r.data) {
		return n, r.err
	}
	return n, nil
}

const contentTypeDetectSize = 512

func newDetector() *detector {
	return &detector{done: make(chan struct{}), data: make([]byte, contentTypeDetectSize)}
}

func (d *detector) Wait() {
	<-d.done
}

func (d *detector) SetReader(reader io.Reader) {
	d.Reader = reader
}

func (d *detector) Detect() {
	n, err := d.Reader.Read(d.data)
	if err != nil && err != io.EOF {
		d.err = err
		return
	}
	d.data = d.data[:n]
	d.contentType = http.DetectContentType(d.data)
	close(d.done)
}

func (d *detector) MultiReader() io.Reader {
	return io.MultiReader(newReader(d.data, d.err), d.Reader)
}

func isValidToken(s string) bool {
	for _, c := range s {
		if c <= ' ' || c > 127 {
			return false
		}
		if strings.ContainsRune("()<>@,;:\\\"/[]?={}", c) {
			return false
		}
	}
	return true
}

func isValidValue(s string) bool {
	for _, c := range s {
		// HTTP specification allows for more technically, but we don't want to escape things.
		if c < ' ' || c > 127 || c == '"' {
			return false
		}
	}
	return true
}

func (r *request) receiveFile(clnt client.Object, objectAddress *object.Address) {
	var (
		err      error
		dis      = "inline"
		start    = time.Now()
		filename string
		obj      *object.Object
	)
	if err = tokens.StoreBearerToken(r.RequestCtx); err != nil {
		r.log.Error("could not fetch and store bearer token", zap.Error(err))
		r.Error("could not fetch and store bearer token", fasthttp.StatusBadRequest)
		return
	}
	readDetector := newDetector()
	options := new(client.GetObjectParams).
		WithAddress(objectAddress).
		WithPayloadReaderHandler(func(reader io.Reader) {
			readDetector.SetReader(reader)
			readDetector.Detect()
		})

	obj, err = clnt.GetObject(
		r.RequestCtx,
		options,
	)
	if err != nil {
		r.log.Error(
			"could not receive object",
			zap.Stringer("elapsed", time.Since(start)),
			zap.Error(err),
		)
		var (
			msg   = fmt.Sprintf("could not receive object: %v", err)
			code  = fasthttp.StatusBadRequest
			cause = err
		)
		for unwrap := errors.Unwrap(err); unwrap != nil; unwrap = errors.Unwrap(cause) {
			cause = unwrap
		}
		if st, ok := status.FromError(cause); ok && st != nil {
			if st.Code() == codes.NotFound {
				code = fasthttp.StatusNotFound
			}
			msg = st.Message()
		}
		r.Error(msg, code)
		return
	}
	if r.Request.URI().QueryArgs().GetBool("download") {
		dis = "attachment"
	}
	r.Response.SetBodyStream(readDetector.MultiReader(), int(obj.PayloadSize()))
	r.Response.Header.Set("Content-Length", strconv.FormatUint(obj.PayloadSize(), 10))
	var contentType string
	for _, attr := range obj.Attributes() {
		key := attr.Key()
		val := attr.Value()
		if !isValidToken(key) || !isValidValue(val) {
			continue
		}
		r.Response.Header.Set("X-Attribute-"+key, val)
		switch key {
		case object.AttributeFileName:
			filename = val
		case object.AttributeTimestamp:
			value, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				r.log.Info("couldn't parse creation date",
					zap.String("key", key),
					zap.String("val", val),
					zap.Error(err))
				continue
			}
			r.Response.Header.Set("Last-Modified",
				time.Unix(value, 0).UTC().Format(http.TimeFormat))
		case object.AttributeContentType:
			contentType = val
		}
	}
	r.Response.Header.Set("x-object-id", obj.ID().String())
	r.Response.Header.Set("x-owner-id", obj.OwnerID().String())
	r.Response.Header.Set("x-container-id", obj.ContainerID().String())

	if len(contentType) == 0 {
		if readDetector.err != nil {
			r.log.Error("could not read object", zap.Error(err))
			r.Error("could not read object", fasthttp.StatusBadRequest)
			return
		}
		readDetector.Wait()
		contentType = readDetector.contentType
	}
	r.SetContentType(contentType)

	r.Response.Header.Set("Content-Disposition", dis+"; filename="+path.Base(filename))
}

func (o objectIDs) Slice() []string {
	res := make([]string, 0, len(o))
	for _, oid := range o {
		res = append(res, oid.String())
	}
	return res
}

// Downloader is a download request handler.
type Downloader struct {
	log  *zap.Logger
	pool pool.Pool
}

// New creates an instance of Downloader using specified options.
func New(ctx context.Context, log *zap.Logger, conns pool.Pool) (*Downloader, error) {
	var err error
	d := &Downloader{log: log, pool: conns}
	if err != nil {
		return nil, fmt.Errorf("failed to get neofs client's reusable artifacts: %w", err)
	}
	return d, nil
}

func (d *Downloader) newRequest(ctx *fasthttp.RequestCtx, log *zap.Logger) *request {
	return &request{
		RequestCtx: ctx,
		log:        log,
	}
}

// DownloadByAddress handles download requests using simple cid/oid format.
func (d *Downloader) DownloadByAddress(c *fasthttp.RequestCtx) {
	var (
		err     error
		address = object.NewAddress()
		cid, _  = c.UserValue("cid").(string)
		oid, _  = c.UserValue("oid").(string)
		val     = strings.Join([]string{cid, oid}, "/")
		log     = d.log.With(zap.String("cid", cid), zap.String("oid", oid))
	)
	if err = address.Parse(val); err != nil {
		log.Error("wrong object address", zap.Error(err))
		c.Error("wrong object address", fasthttp.StatusBadRequest)
		return
	}

	d.newRequest(c, log).receiveFile(d.pool, address)
}

// DownloadByAttribute handles attribute-based download requests.
func (d *Downloader) DownloadByAttribute(c *fasthttp.RequestCtx) {
	var (
		err     error
		scid, _ = c.UserValue("cid").(string)
		key, _  = c.UserValue("attr_key").(string)
		val, _  = c.UserValue("attr_val").(string)
		log     = d.log.With(zap.String("cid", scid), zap.String("attr_key", key), zap.String("attr_val", val))
		ids     []*object.ID
	)
	cid := cid.New()
	if err = cid.Parse(scid); err != nil {
		log.Error("wrong container id", zap.Error(err))
		c.Error("wrong container id", fasthttp.StatusBadRequest)
		return
	}

	options := object.NewSearchFilters()
	options.AddRootFilter()
	options.AddFilter(key, val, object.MatchStringEqual)

	sops := new(client.SearchObjectParams).WithContainerID(cid).WithSearchFilters(options)
	if ids, err = d.pool.SearchObject(c, sops); err != nil {
		log.Error("something went wrong", zap.Error(err))
		c.Error("something went wrong", fasthttp.StatusBadRequest)
		return
	} else if len(ids) == 0 {
		log.Debug("object not found")
		c.Error("object not found", fasthttp.StatusNotFound)
		return
	}
	if len(ids) > 1 {
		log.Debug("found multiple objects",
			zap.Strings("object_ids", objectIDs(ids).Slice()),
			zap.Stringer("show_object_id", ids[0]))
	}
	address := object.NewAddress()
	address.SetContainerID(cid)
	address.SetObjectID(ids[0])

	d.newRequest(c, log).receiveFile(d.pool, address)
}
