package main

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/nspcc-dev/neofs-api-go/pkg/object"
	"github.com/spf13/viper"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type (
	HeaderFilter interface {
		Filter(header *fasthttp.RequestHeader) map[string]string
	}

	headerFilter struct {
		logger  *zap.Logger
		mapping map[string]string
	}
)

const userAttributeHeaderPrefix = "X-Attribute-"

func newHeaderFilter(l *zap.Logger, v *viper.Viper) HeaderFilter {
	filter := &headerFilter{
		logger:  l,
		mapping: make(map[string]string),
	}

	for i := 0; ; i++ {
		index := strconv.Itoa(i)
		key := strings.Join([]string{cfgUploaderHeader, index, cfgUploaderHeaderKey}, ".")
		rep := strings.Join([]string{cfgUploaderHeader, index, cfgUploaderHeaderVal}, ".")

		keyValue := v.GetString(key)
		repValue := v.GetString(rep)

		if keyValue == "" || repValue == "" {
			break
		}

		filter.mapping[keyValue] = repValue

		l.Debug("load upload header table value",
			zap.String("key", keyValue),
			zap.String("val", repValue))
	}

	// Default values
	filter.mapping[object.AttributeFileName] = object.AttributeFileName
	filter.mapping[object.AttributeTimestamp] = object.AttributeTimestamp

	return filter
}

func (h *headerFilter) Filter(header *fasthttp.RequestHeader) map[string]string {
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

		// checks that after removing attribute prefix we had not empty key
		if key = bytes.TrimPrefix(key, prefix); len(key) == 0 {
			return
		}

		// checks mapping table and if we found record store it
		// at resulting hashmap
		if name, ok := h.mapping[string(key)]; ok {
			result[name] = string(val)

			h.logger.Debug("add attribute to result object",
				zap.String("key", name),
				zap.String("val", string(val)))

			return
		}

		// otherwise inform that attribute will be ignored
		h.logger.Debug("ignore attribute",
			zap.String("key", string(key)),
			zap.String("val", string(val)))
	})

	return result
}
