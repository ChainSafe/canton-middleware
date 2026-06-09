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
			name:  "single chunk fits exactly",
			start: 1, end: 5, maxRange: 5,
			want: []blockRange{{1, 5}},
		},
		{
			name:  "range smaller than max",
			start: 10, end: 12, maxRange: 100,
			want: []blockRange{{10, 12}},
		},
		{
			name:  "splits into multiple chunks",
			start: 1, end: 25, maxRange: 10,
			want: []blockRange{{1, 10}, {11, 20}, {21, 25}},
		},
		{
			name:  "exact multiple of maxRange",
			start: 1, end: 30, maxRange: 10,
			want: []blockRange{{1, 10}, {11, 20}, {21, 30}},
		},
		{
			name:  "single block range",
			start: 42, end: 42, maxRange: 100,
			want: []blockRange{{42, 42}},
		},
		{
			name:  "start greater than end returns nil",
			start: 10, end: 5, maxRange: 10,
			want: nil,
		},
		{
			name:  "zero maxRange returns nil",
			start: 1, end: 10, maxRange: 0,
			want: nil,
		},
		{
			name:  "maxRange of one yields single-block chunks",
			start: 7, end: 9, maxRange: 1,
			want: []blockRange{{7, 7}, {8, 8}, {9, 9}},
		},
		{
			name:  "large maxRange avoids overflow",
			start: 2, end: 10, maxRange: ^uint64(0),
			want: []blockRange{{2, 10}},
		},
		{
			name:  "range ending at uint64 max terminates",
			start: ^uint64(0) - 1, end: ^uint64(0), maxRange: 1,
			want: []blockRange{{^uint64(0) - 1, ^uint64(0) - 1}, {^uint64(0), ^uint64(0)}},
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
