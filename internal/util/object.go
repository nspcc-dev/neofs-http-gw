package util

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"time"

	"github.com/nspcc-dev/neofs-sdk-go/client"
	apistatus "github.com/nspcc-dev/neofs-sdk-go/client/status"
	"github.com/nspcc-dev/neofs-sdk-go/netmap"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/valyala/fasthttp"
)

// PrmAttributes groups parameters to form attributes from request headers.
type PrmAttributes struct {
	DefaultTimestamp bool
	DefaultFileName  string
}

type epochDurations struct {
	currentEpoch  uint64
	msPerBlock    int64
	blockPerEpoch uint64
}

const (
	UserAttributeHeaderPrefix = "X-Attribute-"
	SystemAttributePrefix     = "__NEOFS__"

	ExpirationDurationAttr  = SystemAttributePrefix + "EXPIRATION_DURATION"
	ExpirationTimestampAttr = SystemAttributePrefix + "EXPIRATION_TIMESTAMP"
	ExpirationRFC3339Attr   = SystemAttributePrefix + "EXPIRATION_RFC3339"
)

// GetObjectAttributes forms object attributes from request headers.
func GetObjectAttributes(ctx context.Context, header *fasthttp.RequestHeader, pool *pool.Pool, prm PrmAttributes) ([]object.Attribute, error) {
	filtered := filterHeaders(header)
	if needParseExpiration(filtered) {
		epochDuration, err := getEpochDurations(ctx, pool)
		if err != nil {
			return nil, fmt.Errorf("could not get epoch durations from network info: %w", err)
		}
		if err = prepareExpirationHeader(filtered, epochDuration); err != nil {
			return nil, fmt.Errorf("could not prepare expiration header: %w", err)
		}
	}

	attributes := make([]object.Attribute, 0, len(filtered))
	// prepares attributes from filtered headers
	for key, val := range filtered {
		attribute := object.NewAttribute()
		attribute.SetKey(key)
		attribute.SetValue(val)
		attributes = append(attributes, *attribute)
	}
	// sets FileName attribute if it wasn't set from header
	if _, ok := filtered[object.AttributeFileName]; !ok && prm.DefaultFileName != "" {
		filename := object.NewAttribute()
		filename.SetKey(object.AttributeFileName)
		filename.SetValue(prm.DefaultFileName)
		attributes = append(attributes, *filename)
	}
	// sets Timestamp attribute if it wasn't set from header and enabled by settings
	if _, ok := filtered[object.AttributeTimestamp]; !ok && prm.DefaultTimestamp {
		timestamp := object.NewAttribute()
		timestamp.SetKey(object.AttributeTimestamp)
		timestamp.SetValue(strconv.FormatInt(time.Now().Unix(), 10))
		attributes = append(attributes, *timestamp)
	}

	return attributes, nil
}

func getEpochDurations(ctx context.Context, p *pool.Pool) (*epochDurations, error) {
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
	_, ok1 := headers[ExpirationDurationAttr]
	_, ok2 := headers[ExpirationRFC3339Attr]
	_, ok3 := headers[ExpirationTimestampAttr]
	return ok1 || ok2 || ok3
}
