package downloader

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/nspcc-dev/neofs-http-gw/response"
	"github.com/nspcc-dev/neofs-http-gw/tokens"
	"github.com/nspcc-dev/neofs-http-gw/utils"
	"github.com/nspcc-dev/neofs-sdk-go/client"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
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

var errObjectNotFound = errors.New("object not found")

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

func (r request) receiveFile(clnt pool.Object, objectAddress *object.Address) {
	var (
		err      error
		dis      = "inline"
		start    = time.Now()
		filename string
		obj      *object.Object
	)
	if err = tokens.StoreBearerToken(r.RequestCtx); err != nil {
		r.log.Error("could not fetch and store bearer token", zap.Error(err))
		response.Error(r.RequestCtx, "could not fetch and store bearer token", fasthttp.StatusBadRequest)
		return
	}
	readDetector := newDetector()
	options := new(client.GetObjectParams).
		WithAddress(objectAddress).
		WithPayloadReaderHandler(func(reader io.Reader) {
			readDetector.SetReader(reader)
			readDetector.Detect()
		})

	obj, err = clnt.GetObject(r.RequestCtx, options, bearerOpts(r.RequestCtx))
	if err != nil {
		r.handleNeoFSErr(err, start)
		return
	}
	if r.Request.URI().QueryArgs().GetBool("download") {
		dis = "attachment"
	}
	r.Response.SetBodyStream(readDetector.MultiReader(), int(obj.PayloadSize()))
	r.Response.Header.Set(fasthttp.HeaderContentLength, strconv.FormatUint(obj.PayloadSize(), 10))
	var contentType string
	for _, attr := range obj.Attributes() {
		key := attr.Key()
		val := attr.Value()
		if !isValidToken(key) || !isValidValue(val) {
			continue
		}
		if strings.HasPrefix(key, utils.SystemAttributePrefix) {
			key = systemBackwardTranslator(key)
		}
		r.Response.Header.Set(utils.UserAttributeHeaderPrefix+key, val)
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
			r.Response.Header.Set(fasthttp.HeaderLastModified,
				time.Unix(value, 0).UTC().Format(http.TimeFormat))
		case object.AttributeContentType:
			contentType = val
		}
	}
	r.Response.Header.Set(hdrObjectID, obj.ID().String())
	r.Response.Header.Set(hdrOwnerID, obj.OwnerID().String())
	r.Response.Header.Set(hdrContainerID, obj.ContainerID().String())

	if len(contentType) == 0 {
		if readDetector.err != nil {
			r.log.Error("could not read object", zap.Error(err))
			response.Error(r.RequestCtx, "could not read object", fasthttp.StatusBadRequest)
			return
		}
		readDetector.Wait()
		contentType = readDetector.contentType
	}
	r.SetContentType(contentType)

	r.Response.Header.Set(fasthttp.HeaderContentDisposition, dis+"; filename="+path.Base(filename))
}

// systemBackwardTranslator is used to convert headers looking like '__NEOFS__ATTR_NAME' to 'Neofs-Attr-Name'.
func systemBackwardTranslator(key string) string {
	// trim specified prefix '__NEOFS__'
	key = strings.TrimPrefix(key, utils.SystemAttributePrefix)

	var res strings.Builder
	res.WriteString("Neofs-")

	strs := strings.Split(key, "_")
	for i, s := range strs {
		s = strings.Title(strings.ToLower(s))
		res.WriteString(s)
		if i != len(strs)-1 {
			res.WriteString("-")
		}
	}

	return res.String()
}

func bearerOpts(ctx context.Context) pool.CallOption {
	if tkn, err := tokens.LoadBearerToken(ctx); err == nil {
		return pool.WithBearer(tkn)
	}
	return pool.WithBearer(nil)
}

func (r *request) handleNeoFSErr(err error, start time.Time) {
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

	if strings.Contains(cause.Error(), "not found") ||
		strings.Contains(cause.Error(), "can't fetch container info") {
		code = fasthttp.StatusNotFound
		msg = errObjectNotFound.Error()
	}

	response.Error(r.RequestCtx, msg, code)
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
	log      *zap.Logger
	pool     pool.Pool
	settings Settings
}

type Settings struct {
	ZipCompression bool
}

// New creates an instance of Downloader using specified options.
func New(log *zap.Logger, settings Settings, conns pool.Pool) (*Downloader, error) {
	var err error
	d := &Downloader{log: log, pool: conns, settings: settings}
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
	d.byAddress(c, request.receiveFile)
}

// byAddress is wrapper for function (e.g. request.headObject, request.receiveFile) that
// prepares request and object address to it.
func (d *Downloader) byAddress(c *fasthttp.RequestCtx, f func(request, pool.Object, *object.Address)) {
	var (
		address = object.NewAddress()
		cid, _  = c.UserValue("cid").(string)
		oid, _  = c.UserValue("oid").(string)
		val     = strings.Join([]string{cid, oid}, "/")
		log     = d.log.With(zap.String("cid", cid), zap.String("oid", oid))
	)
	if err := address.Parse(val); err != nil {
		log.Error("wrong object address", zap.Error(err))
		response.Error(c, "wrong object address", fasthttp.StatusBadRequest)
		return
	}

	f(*d.newRequest(c, log), d.pool, address)
}

// DownloadByAttribute handles attribute-based download requests.
func (d *Downloader) DownloadByAttribute(c *fasthttp.RequestCtx) {
	d.byAttribute(c, request.receiveFile)
}

// byAttribute is wrapper similar to byAddress.
func (d *Downloader) byAttribute(c *fasthttp.RequestCtx, f func(request, pool.Object, *object.Address)) {
	var (
		httpStatus = fasthttp.StatusBadRequest
		scid, _    = c.UserValue("cid").(string)
		key, _     = url.QueryUnescape(c.UserValue("attr_key").(string))
		val, _     = url.QueryUnescape(c.UserValue("attr_val").(string))
		log        = d.log.With(zap.String("cid", scid), zap.String("attr_key", key), zap.String("attr_val", val))
	)
	containerID := cid.New()
	if err := containerID.Parse(scid); err != nil {
		log.Error("wrong container id", zap.Error(err))
		response.Error(c, "wrong container id", httpStatus)
		return
	}

	address, err := d.searchObject(c, log, containerID, key, val)
	if err != nil {
		log.Error("couldn't search object", zap.Error(err))
		if errors.Is(err, errObjectNotFound) {
			httpStatus = fasthttp.StatusNotFound
		}
		response.Error(c, "couldn't search object", httpStatus)
		return
	}

	f(*d.newRequest(c, log), d.pool, address)
}

func (d *Downloader) searchObject(c *fasthttp.RequestCtx, log *zap.Logger, cid *cid.ID, key, val string) (*object.Address, error) {
	ids, err := d.searchByAttr(c, cid, key, val)
	if err != nil {
		return nil, err
	}
	if len(ids) > 1 {
		log.Debug("found multiple objects",
			zap.Strings("object_ids", objectIDs(ids).Slice()),
			zap.Stringer("show_object_id", ids[0]))
	}

	return formAddress(cid, ids[0]), nil
}

func formAddress(cid *cid.ID, oid *object.ID) *object.Address {
	address := object.NewAddress()
	address.SetContainerID(cid)
	address.SetObjectID(oid)
	return address
}

func (d *Downloader) search(c *fasthttp.RequestCtx, cid *cid.ID, key, val string, op object.SearchMatchType) ([]*object.ID, error) {
	options := object.NewSearchFilters()
	options.AddRootFilter()
	options.AddFilter(key, val, op)

	sops := new(client.SearchObjectParams).WithContainerID(cid).WithSearchFilters(options)
	ids, err := d.pool.SearchObject(c, sops)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errObjectNotFound
	}
	return ids, nil
}

func (d *Downloader) searchByPrefix(c *fasthttp.RequestCtx, cid *cid.ID, val string) ([]*object.ID, error) {
	return d.search(c, cid, object.AttributeFileName, val, object.MatchCommonPrefix)
}

func (d *Downloader) searchByAttr(c *fasthttp.RequestCtx, cid *cid.ID, key, val string) ([]*object.ID, error) {
	return d.search(c, cid, key, val, object.MatchStringEqual)
}

// DownloadZipped handles zip by prefix requests.
func (d *Downloader) DownloadZipped(c *fasthttp.RequestCtx) {
	status := fasthttp.StatusBadRequest
	scid, _ := c.UserValue("cid").(string)
	prefix, _ := url.QueryUnescape(c.UserValue("prefix").(string))
	log := d.log.With(zap.String("cid", scid), zap.String("prefix", prefix))

	containerID := cid.New()
	if err := containerID.Parse(scid); err != nil {
		log.Error("wrong container id", zap.Error(err))
		response.Error(c, "wrong container id", status)
		return
	}

	if err := tokens.StoreBearerToken(c); err != nil {
		log.Error("could not fetch and store bearer token", zap.Error(err))
		response.Error(c, "could not fetch and store bearer token", fasthttp.StatusBadRequest)
		return
	}

	ids, err := d.searchByPrefix(c, containerID, prefix)
	if err != nil {
		log.Error("couldn't find objects", zap.Error(err))
		if errors.Is(err, errObjectNotFound) {
			status = fasthttp.StatusNotFound
		}
		response.Error(c, "couldn't find objects", status)
		return
	}

	c.Response.Header.Set(fasthttp.HeaderContentType, "application/zip")
	c.Response.Header.Set(fasthttp.HeaderContentDisposition, "attachment; filename=\"archive.zip\"")
	c.Response.SetStatusCode(http.StatusOK)

	if err = d.streamFiles(c, containerID, ids); err != nil {
		log.Error("couldn't stream files", zap.Error(err))
		response.Error(c, "couldn't stream", fasthttp.StatusInternalServerError)
		return
	}
}

func (d *Downloader) streamFiles(c *fasthttp.RequestCtx, cid *cid.ID, ids []*object.ID) error {
	zipWriter := zip.NewWriter(c)
	compression := zip.Store
	if d.settings.ZipCompression {
		compression = zip.Deflate
	}

	for _, id := range ids {
		var r io.Reader
		readerInitCtx, initReader := context.WithCancel(c)
		options := new(client.GetObjectParams).
			WithAddress(formAddress(cid, id)).
			WithPayloadReaderHandler(func(reader io.Reader) {
				r = reader
				initReader()
			})

		obj, err := d.pool.GetObject(c, options, bearerOpts(c))
		if err != nil {
			return err
		}

		header := &zip.FileHeader{
			Name:     getFilename(obj),
			Method:   compression,
			Modified: time.Now(),
		}
		entryWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		<-readerInitCtx.Done()
		_, err = io.Copy(entryWriter, r)
		if err != nil {
			return err
		}

		if err = zipWriter.Flush(); err != nil {
			return err
		}
	}

	return zipWriter.Close()
}

func getFilename(obj *object.Object) string {
	for _, attr := range obj.Attributes() {
		if attr.Key() == object.AttributeFileName {
			return attr.Value()
		}
	}

	return ""
}
