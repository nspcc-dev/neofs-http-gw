package util

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/nspcc-dev/neofs-api-go/v2/object"
	"github.com/valyala/fasthttp"
)

var neofsAttributeHeaderPrefixes = [...][]byte{[]byte("Neofs-"), []byte("NEOFS-"), []byte("neofs-")}

func systemTranslator(key, prefix []byte) []byte {
	// replace specified prefix with `__NEOFS__`
	key = bytes.Replace(key, prefix, []byte(SystemAttributePrefix), 1)

	// replace `-` with `_`
	key = bytes.ReplaceAll(key, []byte("-"), []byte("_"))

	// replace with uppercase
	return bytes.ToUpper(key)
}

// FilterHeaders return 'X-Attribute-*' headers.
func FilterHeaders(header *fasthttp.RequestHeader) map[string]string {
	result := make(map[string]string)
	prefix := []byte(UserAttributeHeaderPrefix)

	header.VisitAll(func(key, val []byte) {
		// checks that key and val not empty
		if len(key) == 0 || len(val) == 0 {
			return
		}

		// checks that key has attribute prefix
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

		// checks that attribute key not empty
		if len(key) == 0 {
			return
		}

		// make string representation of key / val
		k, v := string(key), string(val)

		result[k] = v
	})

	return result
}

func prepareExpirationHeader(headers map[string]string, epochDurations *epochDurations) error {
	expirationInEpoch := headers[object.SysAttributeExpEpoch]

	if timeRFC3339, ok := headers[ExpirationRFC3339Attr]; ok {
		expTime, err := time.Parse(time.RFC3339, timeRFC3339)
		if err != nil {
			return fmt.Errorf("couldn't parse value %s of header %s", timeRFC3339, ExpirationRFC3339Attr)
		}

		now := time.Now().UTC()
		if expTime.Before(now) {
			return fmt.Errorf("value %s of header %s must be in the future", timeRFC3339, ExpirationRFC3339Attr)
		}
		updateExpirationHeader(headers, epochDurations, expTime.Sub(now))
		delete(headers, ExpirationRFC3339Attr)
	}

	if timestamp, ok := headers[ExpirationTimestampAttr]; ok {
		value, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			return fmt.Errorf("couldn't parse value %s of header %s", timestamp, ExpirationTimestampAttr)
		}
		expTime := time.Unix(value, 0)

		now := time.Now()
		if expTime.Before(now) {
			return fmt.Errorf("value %s of header %s must be in the future", timestamp, ExpirationTimestampAttr)
		}
		updateExpirationHeader(headers, epochDurations, expTime.Sub(now))
		delete(headers, ExpirationTimestampAttr)
	}

	if duration, ok := headers[ExpirationDurationAttr]; ok {
		expDuration, err := time.ParseDuration(duration)
		if err != nil {
			return fmt.Errorf("couldn't parse value %s of header %s", duration, ExpirationDurationAttr)
		}
		if expDuration <= 0 {
			return fmt.Errorf("value %s of header %s must be positive", expDuration, ExpirationDurationAttr)
		}
		updateExpirationHeader(headers, epochDurations, expDuration)
		delete(headers, ExpirationDurationAttr)
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
