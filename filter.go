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

const userAttributeHeader = "X-Attribute-"

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
	prefix := []byte(userAttributeHeader)

	header.VisitAll(func(key, val []byte) {
		if len(key) == 0 || len(val) == 0 {
			return
		} else if !bytes.HasPrefix(key, prefix) {
			return
		} else if key = bytes.TrimPrefix(key, prefix); len(key) == 0 {
			return
		} else if name, ok := h.mapping[string(key)]; ok {
			result[name] = string(val)

			h.logger.Debug("add attribute to result object",
				zap.String("key", name),
				zap.String("val", string(val)))

			return
		}

		h.logger.Debug("ignore attribute",
			zap.String("key", string(key)),
			zap.String("val", string(val)))
	})

	return result
}
