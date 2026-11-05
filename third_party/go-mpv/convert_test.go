//go:build !cgo || nocgo

package mpv

import "testing"

// stackNoise dirties a chunk of stack so a dangling local is overwritten.
func stackNoise(n int) int {
	var buf [64]int
	sum := n
	for i := range buf {
		buf[i] = n + i
		sum += buf[i]
	}
	if n > 0 {
		sum += stackNoise(n - 1)
	}
	return sum
}

func TestConvertDataFlagOutlivesTheCall(t *testing.T) {
	ptr, cleanup := convertData(FormatFlag, true)
	defer cleanup()
	if ptr == nil {
		t.Fatal("nil flag pointer")
	}
	sink = stackNoise(32)
	if got := *(*int32)(ptr); got != 1 {
		t.Fatalf("pause flag was %d after the caller returned", got)
	}
	off, cleanupOff := convertData(FormatFlag, false)
	defer cleanupOff()
	sink = stackNoise(32)
	if got := *(*int32)(off); got != 0 {
		t.Fatalf("clear flag was %d", got)
	}
}

func TestConvertDataNumberOutlivesTheCall(t *testing.T) {
	ptr, cleanup := convertData(FormatDouble, 1.25)
	defer cleanup()
	sink = stackNoise(32)
	if got := *(*float64)(ptr); got != 1.25 {
		t.Fatalf("double was %v", got)
	}
	ip, iclean := convertData(FormatInt64, int64(42))
	defer iclean()
	sink = stackNoise(32)
	if got := *(*int64)(ip); got != 42 {
		t.Fatalf("int64 was %d", got)
	}
}

// sink keeps stackNoise's result alive so the compiler cannot delete it.
var sink int
