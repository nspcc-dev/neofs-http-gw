package downloader

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReader(t *testing.T) {
	data := []byte("test string")
	err := fmt.Errorf("something wrong")

	for _, tc := range []struct {
		err  error
		buff []byte
	}{
		{err: nil, buff: make([]byte, len(data)+1)},
		{err: nil, buff: make([]byte, len(data))},
		{err: nil, buff: make([]byte, len(data)-1)},
		{err: err, buff: make([]byte, len(data)+1)},
		{err: err, buff: make([]byte, len(data))},
		{err: err, buff: make([]byte, len(data)-1)},
	} {
		var res []byte
		var err error
		var n int

		r := newReader(data, tc.err)
		for err == nil {
			n, err = r.Read(tc.buff)
			res = append(res, tc.buff[:n]...)
		}

		if tc.err == nil {
			require.Equal(t, io.EOF, err)
		} else {
			require.Equal(t, tc.err, err)
		}
		require.Equal(t, data, res)
	}
}
