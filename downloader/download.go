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
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/nspcc-dev/neofs-sdk-go/object/address"
	oid "github.com/nspcc-dev/neofs-sdk-go/object/id"
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

func (r request) receiveFile(clnt pool.Object, objectAddress *address.Address) {
	var (
		err      error
		dis      = "inline"
		start    = time.Now()
		filename string
	)
	if err = tokens.StoreBearerToken(r.RequestCtx); err != nil {
		r.log.Error("could not fetch and store bearer token", zap.Error(err))
		response.Error(r.RequestCtx, "could not fetch and store bearer token", fasthttp.StatusBadRequest)
		return
	}

	rObj, err := clnt.GetObject(r.RequestCtx, *objectAddress, bearerOpts(r.RequestCtx))
	if err != nil {
		r.handleNeoFSErr(err, start)
		return
	}

	// we can't close reader in this function, so how to do it?

	if r.Request.URI().QueryArgs().GetBool("download") {
		dis = "attachment"
	}

	readDetector := newDetector()
	readDetector.SetReader(rObj.Payload)
	readDetector.Detect()

	r.Response.SetBodyStream(readDetector.MultiReader(), int(rObj.Header.PayloadSize()))
	r.Response.Header.Set(fasthttp.HeaderContentLength, strconv.FormatUint(rObj.Header.PayloadSize(), 10))
	var contentType string
	for _, attr := range rObj.Header.Attributes() {
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

	idsToResponse(&r.Response, &rObj.Header)

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
func (d *Downloader) byAddress(c *fasthttp.RequestCtx, f func(request, pool.Object, *address.Address)) {
	var (
		addr     = address.NewAddress()
		idCnr, _ = c.UserValue("cid").(string)
		idObj, _ = c.UserValue("oid").(string)
		val      = strings.Join([]string{idCnr, idObj}, "/")
		log      = d.log.With(zap.String("cid", idCnr), zap.String("oid", idObj))
	)
	if err := addr.Parse(val); err != nil {
		log.Error("wrong object address", zap.Error(err))
		response.Error(c, "wrong object address", fasthttp.StatusBadRequest)
		return
	}

	f(*d.newRequest(c, log), d.pool, addr)
}

// DownloadByAttribute handles attribute-based download requests.
func (d *Downloader) DownloadByAttribute(c *fasthttp.RequestCtx) {
	d.byAttribute(c, request.receiveFile)
}

// byAttribute is wrapper similar to byAddress.
func (d *Downloader) byAttribute(c *fasthttp.RequestCtx, f func(request, pool.Object, *address.Address)) {
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

	res, err := d.search(c, containerID, key, val, object.MatchStringEqual)
	if err != nil {
		log.Error("could not search for objects", zap.Error(err))
		response.Error(c, "could not search for objects", fasthttp.StatusBadRequest)
		return
	}

	defer res.Close()

	buf := make([]oid.ID, 1)

	n, err := res.Read(buf)
	if n == 0 {
		if errors.Is(err, io.EOF) {
			log.Error("object not found", zap.Error(err))
			response.Error(c, "object not found", fasthttp.StatusNotFound)
			return
		}

		log.Error("read object list failed", zap.Error(err))
		response.Error(c, "read object list failed", fasthttp.StatusBadRequest)
		return
	}

	var addrObj address.Address
	addrObj.SetContainerID(containerID)
	addrObj.SetObjectID(&buf[0])

	f(*d.newRequest(c, log), d.pool, &addrObj)
}

func (d *Downloader) search(c *fasthttp.RequestCtx, cid *cid.ID, key, val string, op object.SearchMatchType) (*pool.ResObjectSearch, error) {
	filters := object.NewSearchFilters()
	filters.AddRootFilter()
	filters.AddFilter(key, val, op)

	return d.pool.SearchObjects(c, *cid, filters)
}

// DownloadZipped handles zip by prefix requests.
func (d *Downloader) DownloadZipped(c *fasthttp.RequestCtx) {
	scid, _ := c.UserValue("cid").(string)
	prefix, _ := url.QueryUnescape(c.UserValue("prefix").(string))
	log := d.log.With(zap.String("cid", scid), zap.String("prefix", prefix))

	containerID := cid.New()
	if err := containerID.Parse(scid); err != nil {
		log.Error("wrong container id", zap.Error(err))
		response.Error(c, "wrong container id", fasthttp.StatusBadRequest)
		return
	}

	if err := tokens.StoreBearerToken(c); err != nil {
		log.Error("could not fetch and store bearer token", zap.Error(err))
		response.Error(c, "could not fetch and store bearer token", fasthttp.StatusBadRequest)
		return
	}

	resSearch, err := d.search(c, containerID, object.AttributeFileName, prefix, object.MatchCommonPrefix)
	if err != nil {
		log.Error("could not search for objects", zap.Error(err))
		response.Error(c, "could not search for objects", fasthttp.StatusBadRequest)
		return
	}

	defer resSearch.Close()

	c.Response.Header.Set(fasthttp.HeaderContentType, "application/zip")
	c.Response.Header.Set(fasthttp.HeaderContentDisposition, "attachment; filename=\"archive.zip\"")
	c.Response.SetStatusCode(http.StatusOK)

	zipWriter := zip.NewWriter(c)
	compression := zip.Store
	if d.settings.ZipCompression {
		compression = zip.Deflate
	}

	var (
		addr   address.Address
		resGet *pool.ResGetObject
		w      io.Writer
		bufZip []byte
	)

	addr.SetContainerID(containerID)

	optBearer := bearerOpts(c)
	empty := true
	n := 0
	buf := make([]oid.ID, 10) // configure?

iterator:
	for {
		n, err = resSearch.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if empty {
					log.Error("objects not found", zap.Error(err))
					response.Error(c, "objects not found", fasthttp.StatusNotFound)
					return
				}

				err = nil

				break
			}

			log.Error("read object list failed", zap.Error(err))
			response.Error(c, "read object list failed", fasthttp.StatusBadRequest) // maybe best effort?
			return
		}

		if empty {
			bufZip = make([]byte, 1024) // configure?
		}

		empty = false

		for i := range buf[:n] {
			addr.SetObjectID(&buf[i])

			resGet, err = d.pool.GetObject(c, addr, optBearer)
			if err != nil {
				err = fmt.Errorf("get NeoFS object: %v", err)
				break iterator
			}

			w, err = zipWriter.CreateHeader(&zip.FileHeader{
				Name:     getFilename(&resGet.Header),
				Method:   compression,
				Modified: time.Now(),
			})
			if err != nil {
				err = fmt.Errorf("zip create header: %v", err)
				break iterator
			}

			_, err = io.CopyBuffer(w, resGet.Payload, bufZip)
			if err != nil {
				err = fmt.Errorf("copy object payload to zip file: %v", err)
				break iterator
			}

			_ = resGet.Payload.Close()

			err = zipWriter.Flush()
			if err != nil {
				err = fmt.Errorf("flush zip writer: %v", err)
				break iterator
			}
		}
	}

	if err == nil {
		err = zipWriter.Close()
	}

	if err != nil {
		log.Error("file streaming failure", zap.Error(err))
		response.Error(c, "file streaming failure", fasthttp.StatusInternalServerError)
		return
	}
}

func getFilename(obj *object.Object) string {
	for _, attr := range obj.Attributes() {
		if attr.Key() == object.AttributeFileName {
			return attr.Value()
		}
	}

	return ""
}
