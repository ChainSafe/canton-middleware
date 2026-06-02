// SPDX-License-Identifier: Apache-2.0

package ethereum

import (
	"reflect"
	"testing"
)

func TestChunkRange(t *testing.T) {
	tests := []struct {
		name     string
		start    uint64
		end      uint64
		maxRange uint64
		want     []blockRange
	}{
		{
			name:     "single chunk fits exactly",
			start:    1, end: 5, maxRange: 5,
			want: []blockRange{{1, 5}},
		},
		{
			name:     "range smaller than max",
			start:    10, end: 12, maxRange: 100,
			want: []blockRange{{10, 12}},
		},
		{
			name:     "splits into multiple chunks",
			start:    1, end: 25, maxRange: 10,
			want: []blockRange{{1, 10}, {11, 20}, {21, 25}},
		},
		{
			name:     "exact multiple of maxRange",
			start:    1, end: 30, maxRange: 10,
			want: []blockRange{{1, 10}, {11, 20}, {21, 30}},
		},
		{
			name:     "single block range",
			start:    42, end: 42, maxRange: 100,
			want: []blockRange{{42, 42}},
		},
		{
			name:     "start greater than end returns nil",
			start:    10, end: 5, maxRange: 10,
			want: nil,
		},
		{
			name:     "zero maxRange returns nil",
			start:    1, end: 10, maxRange: 0,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkRange(tt.start, tt.end, tt.maxRange)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("chunkRange(%d,%d,%d) = %v, want %v",
					tt.start, tt.end, tt.maxRange, got, tt.want)
			}
		})
	}
}
