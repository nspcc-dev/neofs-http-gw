package downloader

import (
	"archive/zip"
	"bytes"
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
	"github.com/nspcc-dev/neofs-sdk-go/token"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type request struct {
	*fasthttp.RequestCtx
	log *zap.Logger
}

var errObjectNotFound = errors.New("object not found")

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

type readCloser struct {
	io.Reader
	io.Closer
}

// initializes io.Reader with limited size and detects Content-Type from it.
// Returns r's error directly. Also returns processed data.
func readContentType(maxSize uint64, rInit func(uint64) (io.Reader, error)) (string, []byte, error) {
	if maxSize > sizeToDetectType {
		maxSize = sizeToDetectType
	}

	buf := make([]byte, maxSize) // maybe sync-pool the slice?

	r, err := rInit(maxSize)
	if err != nil {
		return "", nil, err
	}

	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return "", nil, err
	}

	buf = buf[:n]

	return http.DetectContentType(buf), buf, err // to not lose io.EOF
}

func (r request) receiveFile(clnt *pool.Pool, objectAddress *address.Address) {
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

	var prm pool.PrmObjectGet
	prm.SetAddress(*objectAddress)
	prm.UseBearer(bearerToken(r.RequestCtx))

	rObj, err := clnt.GetObject(r.RequestCtx, prm)
	if err != nil {
		r.handleNeoFSErr(err, start)
		return
	}

	// we can't close reader in this function, so how to do it?

	if r.Request.URI().QueryArgs().GetBool("download") {
		dis = "attachment"
	}

	payloadSize := rObj.Header.PayloadSize()

	r.Response.Header.Set(fasthttp.HeaderContentLength, strconv.FormatUint(payloadSize, 10))
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
		// determine the Content-Type from the payload head
		var payloadHead []byte

		contentType, payloadHead, err = readContentType(payloadSize, func(uint64) (io.Reader, error) {
			return rObj.Payload, nil
		})
		if err != nil && err != io.EOF {
			r.log.Error("could not detect Content-Type from payload", zap.Error(err))
			response.Error(r.RequestCtx, "could not detect Content-Type from payload", fasthttp.StatusBadRequest)
			return
		}

		// reset payload reader since part of the data has been read
		var headReader io.Reader = bytes.NewReader(payloadHead)

		if err != io.EOF { // otherwise, we've already read full payload
			headReader = io.MultiReader(headReader, rObj.Payload)
		}

		// note: we could do with io.Reader, but SetBodyStream below closes body stream
		// if it implements io.Closer and that's useful for us.
		rObj.Payload = readCloser{headReader, rObj.Payload}
	}
	r.SetContentType(contentType)

	r.Response.Header.Set(fasthttp.HeaderContentDisposition, dis+"; filename="+path.Base(filename))

	r.Response.SetBodyStream(rObj.Payload, int(payloadSize))
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

func bearerToken(ctx context.Context) *token.BearerToken {
	if tkn, err := tokens.LoadBearerToken(ctx); err == nil {
		return tkn
	}
	return nil
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
	pool     *pool.Pool
	settings Settings
}

type Settings struct {
	ZipCompression bool
}

// New creates an instance of Downloader using specified options.
func New(log *zap.Logger, settings Settings, conns *pool.Pool) (*Downloader, error) {
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
func (d *Downloader) byAddress(c *fasthttp.RequestCtx, f func(request, *pool.Pool, *address.Address)) {
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
func (d *Downloader) byAttribute(c *fasthttp.RequestCtx, f func(request, *pool.Pool, *address.Address)) {
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

	var prm pool.PrmObjectSearch
	prm.SetContainerID(*cid)
	prm.SetFilters(filters)
	prm.UseBearer(bearerToken(c))

	return d.pool.SearchObjects(c, prm)
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

	btoken := bearerToken(c)
	empty := true
	called := false

	errIter := resSearch.Iterate(func(id oid.ID) bool {
		called = true

		if empty {
			bufZip = make([]byte, 1024) // configure?
		}

		empty = false

		addr.SetObjectID(&id)

		var prm pool.PrmObjectGet
		prm.SetAddress(addr)
		prm.UseBearer(btoken)

		resGet, err = d.pool.GetObject(c, prm)
		if err != nil {
			err = fmt.Errorf("get NeoFS object: %v", err)
			return true
		}

		w, err = zipWriter.CreateHeader(&zip.FileHeader{
			Name:     getFilename(&resGet.Header),
			Method:   compression,
			Modified: time.Now(),
		})
		if err != nil {
			err = fmt.Errorf("zip create header: %v", err)
			return true
		}

		_, err = io.CopyBuffer(w, resGet.Payload, bufZip)
		if err != nil {
			err = fmt.Errorf("copy object payload to zip file: %v", err)
			return true
		}

		_ = resGet.Payload.Close()

		err = zipWriter.Flush()
		if err != nil {
			err = fmt.Errorf("flush zip writer: %v", err)
			return true
		}

		return false
	})
	if errIter != nil {
		log.Error("iterating over selected objects failed", zap.Error(errIter))
		response.Error(c, "iterating over selected objects", fasthttp.StatusBadRequest)
		return
	} else if !called {
		log.Error("objects not found")
		response.Error(c, "objects not found", fasthttp.StatusNotFound)
		return
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
