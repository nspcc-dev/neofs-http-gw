package uploader

import (
	"strconv"
	"testing"
	"time"

	"github.com/nspcc-dev/neofs-api-go/v2/object"

	"github.com/nspcc-dev/neofs-sdk-go/logger"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestFilter(t *testing.T) {
	log, err := logger.New()
	require.NoError(t, err)

	req := &fasthttp.RequestHeader{}
	req.DisableNormalizing()
	req.Set("X-Attribute-Neofs-Expiration-Epoch1", "101")
	req.Set("X-Attribute-NEOFS-Expiration-Epoch2", "102")
	req.Set("X-Attribute-neofs-Expiration-Epoch3", "103")
	req.Set("X-Attribute-MyAttribute", "value")

	expected := map[string]string{
		"__NEOFS__EXPIRATION_EPOCH1": "101",
		"MyAttribute":                "value",
		"__NEOFS__EXPIRATION_EPOCH3": "103",
		"__NEOFS__EXPIRATION_EPOCH2": "102",
	}

	result := filterHeaders(log, req)

	require.Equal(t, expected, result)
}

func TestPrepareExpirationHeader(t *testing.T) {
	tomorrow := time.Now().Add(24 * time.Hour)
	tomorrowUnix := tomorrow.Unix()
	tomorrowUnixNano := tomorrow.UnixNano()
	tomorrowUnixMilli := tomorrowUnixNano / 1e6

	epoch := "100"
	duration := "24h"
	timestampSec := strconv.FormatInt(tomorrowUnix, 10)
	timestampMilli := strconv.FormatInt(tomorrowUnixMilli, 10)
	timestampNano := strconv.FormatInt(tomorrowUnixNano, 10)

	defaultDurations := &epochDurations{
		currentEpoch:  10,
		msPerBlock:    1000,
		blockPerEpoch: 101,
	}

	epochPerDay := (24 * time.Hour).Milliseconds() / int64(defaultDurations.blockPerEpoch) / defaultDurations.msPerBlock
	defaultExpEpoch := strconv.FormatInt(int64(defaultDurations.currentEpoch)+epochPerDay, 10)

	for _, tc := range []struct {
		name      string
		headers   map[string]string
		durations *epochDurations
		err       bool
		expected  map[string]string
	}{
		{
			name:     "valid epoch",
			headers:  map[string]string{object.SysAttributeExpEpoch: epoch},
			expected: map[string]string{object.SysAttributeExpEpoch: epoch},
		},
		{
			name: "valid epoch, valid duration",
			headers: map[string]string{
				object.SysAttributeExpEpoch: epoch,
				expirationDurationAttr:      duration,
			},
			durations: defaultDurations,
			expected:  map[string]string{object.SysAttributeExpEpoch: epoch},
		},
		{
			name: "valid epoch, valid rfc3339",
			headers: map[string]string{
				object.SysAttributeExpEpoch: epoch,
				expirationRFC3339Attr:       tomorrow.Format(time.RFC3339),
			},
			durations: defaultDurations,
			expected:  map[string]string{object.SysAttributeExpEpoch: epoch},
		},
		{
			name: "valid epoch, valid timestamp sec",
			headers: map[string]string{
				object.SysAttributeExpEpoch: epoch,
				expirationTimestampAttr:     timestampSec,
			},
			durations: defaultDurations,
			expected:  map[string]string{object.SysAttributeExpEpoch: epoch},
		},
		{
			name: "valid epoch, valid timestamp milli",
			headers: map[string]string{
				object.SysAttributeExpEpoch: epoch,
				expirationTimestampAttr:     timestampMilli,
			},
			durations: defaultDurations,
			expected:  map[string]string{object.SysAttributeExpEpoch: epoch},
		},
		{
			name: "valid epoch, valid timestamp nano",
			headers: map[string]string{
				object.SysAttributeExpEpoch: epoch,
				expirationTimestampAttr:     timestampNano,
			},
			durations: defaultDurations,
			expected:  map[string]string{object.SysAttributeExpEpoch: epoch},
		},
		{
			name:      "valid timestamp sec",
			headers:   map[string]string{expirationTimestampAttr: timestampSec},
			durations: defaultDurations,
			expected:  map[string]string{object.SysAttributeExpEpoch: defaultExpEpoch},
		},
		{
			name:      "valid duration",
			headers:   map[string]string{expirationDurationAttr: duration},
			durations: defaultDurations,
			expected:  map[string]string{object.SysAttributeExpEpoch: defaultExpEpoch},
		},
		{
			name:      "valid rfc3339",
			headers:   map[string]string{expirationRFC3339Attr: tomorrow.Format(time.RFC3339)},
			durations: defaultDurations,
			expected:  map[string]string{object.SysAttributeExpEpoch: defaultExpEpoch},
		},
		{
			name:    "invalid timestamp sec",
			headers: map[string]string{expirationTimestampAttr: "abc"},
			err:     true,
		},
		{
			name:    "invalid timestamp sec zero",
			headers: map[string]string{expirationTimestampAttr: "0"},
			err:     true,
		},
		{
			name:    "invalid duration",
			headers: map[string]string{expirationDurationAttr: "1d"},
			err:     true,
		},
		{
			name:    "invalid duration negative",
			headers: map[string]string{expirationDurationAttr: "-5h"},
			err:     true,
		},
		{
			name:    "invalid rfc3339",
			headers: map[string]string{expirationRFC3339Attr: "abc"},
			err:     true,
		},
		{
			name:    "invalid rfc3339 zero",
			headers: map[string]string{expirationRFC3339Attr: time.RFC3339},
			err:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := prepareExpirationHeader(tc.headers, tc.durations)
			if tc.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, tc.headers)
			}
		})
	}
}
