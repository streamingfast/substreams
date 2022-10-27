package marshaller

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func Benchmark_Marshall(b *testing.B) {
	counter := atomic.NewUint64(0)
	keyGen := func() string {
		address1 := make([]byte, 20)
		binary.LittleEndian.PutUint64(address1, counter.Inc())

		address2 := make([]byte, 20)
		binary.LittleEndian.PutUint64(address2, counter.Inc())

		return fmt.Sprintf("total:%x:%x", address1, address2)
	}

	for _, m := range []Marshaller{&Binary{}, &Proto{}, &ProtoingFast{}} {
		for _, keyCount := range []int{10, 100, 10_000, 100_000, 1_000_000, 10_000_000} {
			runKey := fmt.Sprintf("%d_keys_%T", keyCount, m)
			b.Run(runKey, func(bb *testing.B) {
				s := &StoreData{Kv: map[string][]byte{}}

				bb.StopTimer()
				for i := 0; i < keyCount; i++ {
					s.Kv[keyGen()] = []byte(strconv.FormatInt(int64(i), 10))

					// Not sure why but trying to `set` 1M keys takes more than 11m!
					// s.baseStore.set(uint64(i), keyGen(), []byte(strconv.FormatInt(int64(i), 10)))
				}

				bb.ResetTimer()
				bb.StartTimer()
				bb.ReportAllocs()

				for n := 0; n < bb.N; n++ {
					_, err := m.Marshal(s)
					require.NoError(bb, err)
				}
			})
		}
	}
}
