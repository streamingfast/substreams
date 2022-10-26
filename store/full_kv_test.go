package store

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"testing"
	"time"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func BenchmarkFullKV_Marshall(b *testing.B) {
	counter := atomic.NewUint64(0)
	keyGen := func() string {
		address1 := make([]byte, 20)
		binary.LittleEndian.PutUint64(address1, counter.Inc())

		address2 := make([]byte, 20)
		binary.LittleEndian.PutUint64(address2, counter.Inc())

		return fmt.Sprintf("total:%x:%x", address1, address2)
	}

	for _, keyCount := range []int{10, 100, 10_000, 100_000} {
		b.Run(fmt.Sprintf("%d_keys", keyCount), func(bb *testing.B) {
			s := &FullKV{
				baseStore: newTestBaseStore(bb, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "int64", nil, &BinaryMarshaller{}),
			}

			startTime := time.Now()
			for i := 0; i < keyCount; i++ {
				s.baseStore.set(uint64(i), keyGen(), []byte(strconv.FormatInt(int64(i), 10)))
			}

			if keyCount >= 100_000 {
				fmt.Printf("\nTime elapsed to bootstrap %d keys is %s\n", keyCount, time.Since(startTime))
			}

			for n := 0; n < bb.N; n++ {
				_, _, err := s.Save(uint64(keyCount))
				require.NoError(bb, err)
			}
		})
	}
}
