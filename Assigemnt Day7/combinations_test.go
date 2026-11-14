package combination

import (
	"reflect"
	"testing"
)

func TestCombine(t *testing.T) {
	tests := []struct {
		name     string
		n, k     int
		expected [][]int
	}{
		{"basic n=4,k=2", 4, 2, [][]int{{1, 2}, {1, 3}, {1, 4}, {2, 3}, {2, 4}, {3, 4}}},
		{"single elements k=1", 3, 1, [][]int{{1}, {2}, {3}}},
		{"full combination n=k", 3, 3, [][]int{{1, 2, 3}}},
		{"empty result k=0", 5, 0, [][]int{}},
		{"invalid n=0", 0, 1, [][]int{}},
		{"larger case n=5,k=3", 5, 3, [][]int{
			{1, 2, 3}, {1, 2, 4}, {1, 2, 5},
			{1, 3, 4}, {1, 3, 5}, {1, 4, 5},
			{2, 3, 4}, {2, 3, 5}, {2, 4, 5},
			{3, 4, 5},
		}},
		{"k greater than n", 3, 5, [][]int{}},
		{"negative k", 3, -1, [][]int{}},
		{"negative n", -3, 2, [][]int{}},
		{"n=1,k=1", 1, 1, [][]int{{1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := combine(tt.n, tt.k)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("combine(%d, %d) = %v, want %v", tt.n, tt.k, got, tt.expected)
			}
		})
	}
}

func TestCombineCount(t *testing.T) {
	// C(10,3) = 120
	if got := len(combine(10, 3)); got != 120 {
		t.Errorf("len(combine(10, 3)) = %d, want 120", got)
	}
}

func BenchmarkCombine(b *testing.B) {
	benchmarks := []struct {
		name string
		n, k int
	}{
		{"n=5,k=2", 5, 2},
		{"n=10,k=3", 10, 3},
		{"n=15,k=4", 15, 4},
		{"n=20,k=5", 20, 5},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				combine(bm.n, bm.k)
			}
		})
	}
}
