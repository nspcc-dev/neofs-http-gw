package uploader

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/nspcc-dev/neofs-api-go/v2/object"
	"github.com/nspcc-dev/neofs-http-gw/utils"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

var neofsAttributeHeaderPrefixes = [...][]byte{[]byte("Neofs-"), []byte("NEOFS-"), []byte("neofs-")}

func systemTranslator(key, prefix []byte) []byte {
	// replace the specified prefix with `__NEOFS__`
	key = bytes.Replace(key, prefix, []byte(utils.SystemAttributePrefix), 1)

	// replace `-` with `_`
	key = bytes.ReplaceAll(key, []byte("-"), []byte("_"))

	// replace with uppercase
	return bytes.ToUpper(key)
}

func filterHeaders(l *zap.Logger, header *fasthttp.RequestHeader) map[string]string {
	result := make(map[string]string)
	prefix := []byte(utils.UserAttributeHeaderPrefix)

	header.VisitAll(func(key, val []byte) {
		// checks that the key and the val not empty
		if len(key) == 0 || len(val) == 0 {
			return
		}

		// checks that the key has attribute prefix
		if !bytes.HasPrefix(key, prefix) {
			return
		}

		// removing attribute prefix
		key = bytes.TrimPrefix(key, prefix)

		// checks that it's a system NeoFS header
		for _, system := range neofsAttributeHeaderPrefixes {
			if bytes.HasPrefix(key, system) {
				key = systemTranslator(key, system)
				break
			}
		}

		// checks that the attribute key is not empty
		if len(key) == 0 {
			return
		}

		// make string representation of key / val
		k, v := string(key), string(val)

		result[k] = v

		l.Debug("add attribute to result object",
			zap.String("key", k),
			zap.String("val", v))
	})

	return result
}

func prepareExpirationHeader(headers map[string]string, epochDurations *epochDurations) error {
	expirationInEpoch := headers[object.SysAttributeExpEpoch]

	if timeRFC3339, ok := headers[utils.ExpirationRFC3339Attr]; ok {
		expTime, err := time.Parse(time.RFC3339, timeRFC3339)
		if err != nil {
			return fmt.Errorf("couldn't parse value %s of header %s", timeRFC3339, utils.ExpirationRFC3339Attr)
		}

		now := time.Now().UTC()
		if expTime.Before(now) {
			return fmt.Errorf("value %s of header %s must be in the future", timeRFC3339, utils.ExpirationRFC3339Attr)
		}
		updateExpirationHeader(headers, epochDurations, expTime.Sub(now))
		delete(headers, utils.ExpirationRFC3339Attr)
	}

	if timestamp, ok := headers[utils.ExpirationTimestampAttr]; ok {
		value, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return fmt.Errorf("couldn't parse value %s of header %s", timestamp, utils.ExpirationTimestampAttr)
		}
		expTime := time.Unix(value, 0)

		now := time.Now()
		if expTime.Before(now) {
			return fmt.Errorf("value %s of header %s must be in the future", timestamp, utils.ExpirationTimestampAttr)
		}
		updateExpirationHeader(headers, epochDurations, expTime.Sub(now))
		delete(headers, utils.ExpirationTimestampAttr)
	}

	if duration, ok := headers[utils.ExpirationDurationAttr]; ok {
		expDuration, err := time.ParseDuration(duration)
		if err != nil {
			return fmt.Errorf("couldn't parse value %s of header %s", duration, utils.ExpirationDurationAttr)
		}
		if expDuration <= 0 {
			return fmt.Errorf("value %s of header %s must be positive", expDuration, utils.ExpirationDurationAttr)
		}
		updateExpirationHeader(headers, epochDurations, expDuration)
		delete(headers, utils.ExpirationDurationAttr)
	}

	if expirationInEpoch != "" {
		headers[object.SysAttributeExpEpoch] = expirationInEpoch
	}

	return nil
}

func updateExpirationHeader(headers map[string]string, durations *epochDurations, expDuration time.Duration) {
	epochDuration := durations.msPerBlock * int64(durations.blockPerEpoch)
	numEpoch := expDuration.Milliseconds() / epochDuration
	headers[object.SysAttributeExpEpoch] = strconv.FormatInt(int64(durations.currentEpoch)+numEpoch, 10)
}
