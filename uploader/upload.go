package uploader

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/nspcc-dev/neofs-http-gw/response"
	"github.com/nspcc-dev/neofs-http-gw/tokens"
	"github.com/nspcc-dev/neofs-http-gw/utils"
	"github.com/nspcc-dev/neofs-sdk-go/client"
	apistatus "github.com/nspcc-dev/neofs-sdk-go/client/status"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/netmap"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/nspcc-dev/neofs-sdk-go/object/address"
	oid "github.com/nspcc-dev/neofs-sdk-go/object/id"
	"github.com/nspcc-dev/neofs-sdk-go/owner"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/nspcc-dev/neofs-sdk-go/token"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

const (
	jsonHeader   = "application/json; charset=UTF-8"
	drainBufSize = 4096
)

// Uploader is an upload request handler.
type Uploader struct {
	log                    *zap.Logger
	pool                   pool.Pool
	enableDefaultTimestamp bool
}

type epochDurations struct {
	currentEpoch  uint64
	msPerBlock    int64
	blockPerEpoch uint64
}

// New creates a new Uploader using specified logger, connection pool and
// other options.
func New(log *zap.Logger, conns pool.Pool, enableDefaultTimestamp bool) *Uploader {
	return &Uploader{log, conns, enableDefaultTimestamp}
}

// Upload handles multipart upload request.
func (u *Uploader) Upload(c *fasthttp.RequestCtx) {
	var (
		err        error
		file       MultipartFile
		obj        *oid.ID
		addr       = address.NewAddress()
		idCnr      = cid.New()
		scid, _    = c.UserValue("cid").(string)
		log        = u.log.With(zap.String("cid", scid))
		bodyStream = c.RequestBodyStream()
		drainBuf   = make([]byte, drainBufSize)
	)
	if err = tokens.StoreBearerToken(c); err != nil {
		log.Error("could not fetch bearer token", zap.Error(err))
		response.Error(c, "could not fetch bearer token", fasthttp.StatusBadRequest)
		return
	}
	if err = idCnr.Parse(scid); err != nil {
		log.Error("wrong container id", zap.Error(err))
		response.Error(c, "wrong container id", fasthttp.StatusBadRequest)
		return
	}
	defer func() {
		// If the temporary reader can be closed - let's close it.
		if file == nil {
			return
		}
		err := file.Close()
		log.Debug(
			"close temporary multipart/form file",
			zap.Stringer("address", addr),
			zap.String("filename", file.FileName()),
			zap.Error(err),
		)
	}()
	boundary := string(c.Request.Header.MultipartFormBoundary())
	if file, err = fetchMultipartFile(u.log, bodyStream, boundary); err != nil {
		log.Error("could not receive multipart/form", zap.Error(err))
		response.Error(c, "could not receive multipart/form: "+err.Error(), fasthttp.StatusBadRequest)
		return
	}
	filtered := filterHeaders(u.log, &c.Request.Header)
	if needParseExpiration(filtered) {
		epochDuration, err := getEpochDurations(c, u.pool)
		if err != nil {
			log.Error("could not get epoch durations from network info", zap.Error(err))
			response.Error(c, "could parse expiration header, try expiration in epoch", fasthttp.StatusBadRequest)
			return
		}
		if err = prepareExpirationHeader(filtered, epochDuration); err != nil {
			log.Error("could not prepare expiration header", zap.Error(err))
			response.Error(c, "could parse expiration header, try expiration in epoch", fasthttp.StatusBadRequest)
			return
		}
	}

	attributes := make([]*object.Attribute, 0, len(filtered))
	// prepares attributes from filtered headers
	for key, val := range filtered {
		attribute := object.NewAttribute()
		attribute.SetKey(key)
		attribute.SetValue(val)
		attributes = append(attributes, attribute)
	}
	// sets FileName attribute if it wasn't set from header
	if _, ok := filtered[object.AttributeFileName]; !ok {
		filename := object.NewAttribute()
		filename.SetKey(object.AttributeFileName)
		filename.SetValue(file.FileName())
		attributes = append(attributes, filename)
	}
	// sets Timestamp attribute if it wasn't set from header and enabled by settings
	if _, ok := filtered[object.AttributeTimestamp]; !ok && u.enableDefaultTimestamp {
		timestamp := object.NewAttribute()
		timestamp.SetKey(object.AttributeTimestamp)
		timestamp.SetValue(strconv.FormatInt(time.Now().Unix(), 10))
		attributes = append(attributes, timestamp)
	}
	id, bt := u.fetchOwnerAndBearerToken(c)

	rawObject := object.NewRaw()
	rawObject.SetContainerID(idCnr)
	rawObject.SetOwnerID(id)
	rawObject.SetAttributes(attributes...)

	if obj, err = u.pool.PutObject(c, *rawObject.Object(), file, pool.WithBearer(bt)); err != nil {
		log.Error("could not store file in neofs", zap.Error(err))
		response.Error(c, "could not store file in neofs", fasthttp.StatusBadRequest)
		return
	}

	addr.SetObjectID(obj)
	addr.SetContainerID(idCnr)

	// Try to return the response, otherwise, if something went wrong, throw an error.
	if err = newPutResponse(addr).encode(c); err != nil {
		log.Error("could not prepare response", zap.Error(err))
		response.Error(c, "could not prepare response", fasthttp.StatusBadRequest)

		return
	}
	// Multipart is multipart and thus can contain more than one part which
	// we ignore at the moment. Also, when dealing with chunked encoding
	// the last zero-length chunk might be left unread (because multipart
	// reader only cares about its boundary and doesn't look further) and
	// it will be (erroneously) interpreted as the start of the next
	// pipelined header. Thus we need to drain the body buffer.
	for {
		_, err = bodyStream.Read(drainBuf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}
	// Report status code and content type.
	c.Response.SetStatusCode(fasthttp.StatusOK)
	c.Response.Header.SetContentType(jsonHeader)
}

func (u *Uploader) fetchOwnerAndBearerToken(ctx context.Context) (*owner.ID, *token.BearerToken) {
	if tkn, err := tokens.LoadBearerToken(ctx); err == nil && tkn != nil {
		return tkn.Issuer(), tkn
	}
	return u.pool.OwnerID(), nil
}

type putResponse struct {
	ObjectID    string `json:"object_id"`
	ContainerID string `json:"container_id"`
}

func newPutResponse(addr *address.Address) *putResponse {
	return &putResponse{
		ObjectID:    addr.ObjectID().String(),
		ContainerID: addr.ContainerID().String(),
	}
}

func (pr *putResponse) encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(pr)
}

func getEpochDurations(ctx context.Context, p pool.Pool) (*epochDurations, error) {
	if conn, _, err := p.Connection(); err != nil {
		return nil, err
	} else if networkInfoRes, err := conn.NetworkInfo(ctx, client.PrmNetworkInfo{}); err != nil {
		return nil, err
	} else if err = apistatus.ErrFromStatus(networkInfoRes.Status()); err != nil {
		return nil, err
	} else {
		networkInfo := networkInfoRes.Info()
		res := &epochDurations{
			currentEpoch: networkInfo.CurrentEpoch(),
			msPerBlock:   networkInfo.MsPerBlock(),
		}

		networkInfo.NetworkConfig().IterateParameters(func(parameter *netmap.NetworkParameter) bool {
			if string(parameter.Key()) == "EpochDuration" {
				data := make([]byte, 8)
				copy(data, parameter.Value())
				res.blockPerEpoch = binary.LittleEndian.Uint64(data)
				return true
			}
			return false
		})
		if res.blockPerEpoch == 0 {
			return nil, fmt.Errorf("not found param: EpochDuration")
		}
		return res, nil
	}
}

func needParseExpiration(headers map[string]string) bool {
	_, ok1 := headers[utils.ExpirationDurationAttr]
	_, ok2 := headers[utils.ExpirationRFC3339Attr]
	_, ok3 := headers[utils.ExpirationTimestampAttr]
	return ok1 || ok2 || ok3
}
