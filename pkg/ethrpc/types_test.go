// SPDX-License-Identifier: Apache-2.0

package ethrpc_test

import (
	"encoding/json"
	"testing"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockTag_UnmarshalJSON(t *testing.T) {
	zero, fortyTwo := uint64(0), uint64(42)
	cases := []struct {
		in      string
		want    *uint64
		wantErr bool
	}{
		{in: `"latest"`},
		{in: `"pending"`},
		{in: `"safe"`},
		{in: `"finalized"`},
		{in: `"earliest"`, want: &zero},
		{in: `"0x0"`, want: &zero},
		{in: `"0x2a"`, want: &fortyTwo},
		{in: `""`, wantErr: true},
		{in: `"42"`, wantErr: true},
		{in: `"head"`, wantErr: true},
		{in: `42`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			var tag ethrpc.BlockTag
			err := json.Unmarshal([]byte(tc.in), &tag)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, tag.Number)
		})
	}
}
