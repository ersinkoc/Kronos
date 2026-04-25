package bench

import "testing"

func BenchmarkTrivial(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = i
	}
}
