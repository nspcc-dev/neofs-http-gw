package uploader

import (
	"bytes"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

const (
	userAttributeHeaderPrefix = "X-Attribute-"
	systemAttributePrefix     = "__NEOFS__"
)

var neofsAttributeHeaderPrefixes = [...][]byte{[]byte("Neofs-"), []byte("NEOFS-"), []byte("neofs-")}

func systemTranslator(key, prefix []byte) []byte {
	// replace specified prefix with `__NEOFS__`
	key = bytes.Replace(key, prefix, []byte(systemAttributePrefix), 1)

	// replace `-` with `_`
	key = bytes.ReplaceAll(key, []byte("-"), []byte("_"))

	// replace with uppercase
	return bytes.ToUpper(key)
}

func filterHeaders(l *zap.Logger, header *fasthttp.RequestHeader) map[string]string {
	result := make(map[string]string)
	prefix := []byte(userAttributeHeaderPrefix)

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

		l.Debug("add attribute to result object",
			zap.String("key", k),
			zap.String("val", v))
	})

	return result
}
