package pboutput

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strconv"
	"testing"
	"time"
)

//var ranges = []int{10_000, 100_000, 1_000_000, 10_000_000}
var ranges = []int{10_000_000}

func Benchmark_Marshall_Map(b *testing.B) {
	for _, keyCount := range ranges {
		runKey := fmt.Sprintf("%d_keys", keyCount)
		b.Run(runKey, func(bb *testing.B) {
			s := &OutputData{
				Kv: make(map[string]*CacheItem),
			}

			bb.StopTimer()
			for i := 0; i < keyCount; i++ {
				s.Kv[uuid.NewString()] = &CacheItem{
					BlockNum:  3,
					BlockId:   uuid.NewString(),
					Payload:   []byte(strconv.FormatInt(int64(i), 10)),
					Timestamp: timestamppb.New(time.Now()),
					Cursor:    uuid.NewString(),
				}
			}

			bb.ResetTimer()
			bb.StartTimer()
			bb.ReportAllocs()

			for n := 0; n < bb.N; n++ {
				data, err := s.MarshalVT()
				require.NoError(bb, err)
				fmt.Println("buf size", len(data))
			}
		})
	}
}

func Benchmark_Unmarshall_Map(b *testing.B) {
	for _, keyCount := range ranges {
		runKey := fmt.Sprintf("%d_keys", keyCount)
		b.Run(runKey, func(bb *testing.B) {
			s := &OutputData{
				Kv: make(map[string]*CacheItem),
			}

			bb.StopTimer()
			for i := 0; i < keyCount; i++ {
				s.Kv[uuid.NewString()] = &CacheItem{
					BlockNum:  3,
					BlockId:   uuid.NewString(),
					Payload:   []byte(strconv.FormatInt(int64(i), 10)),
					Timestamp: timestamppb.New(time.Now()),
					Cursor:    uuid.NewString(),
				}
			}

			data, err := s.MarshalVT()
			require.NoError(bb, err)

			bb.ResetTimer()
			bb.StartTimer()
			bb.ReportAllocs()

			for n := 0; n < bb.N; n++ {
				o := &OutputData{}
				err := o.UnmarshalVTNoAlloc(data)
				require.NoError(bb, err)
			}
		})
	}
}

func Benchmark_Marshall_Array(b *testing.B) {
	for _, keyCount := range ranges {
		runKey := fmt.Sprintf("%d_keys", keyCount)
		b.Run(runKey, func(bb *testing.B) {
			items := make(map[string]*CacheItem)

			bb.StopTimer()
			for i := 0; i < keyCount; i++ {
				items[uuid.NewString()] = &CacheItem{
					BlockNum:  3,
					BlockId:   uuid.NewString(),
					Payload:   []byte(strconv.FormatInt(int64(i), 10)),
					Timestamp: timestamppb.New(time.Now()),
					Cursor:    uuid.NewString(),
				}

			}

			bb.ResetTimer()
			bb.StartTimer()
			bb.ReportAllocs()

			s := &OutputDataTest{
				Items: make([]*CacheItem, len(items)),
			}
			i := 0
			for _, item := range items {
				s.Items[i] = item
				i++
			}

			for n := 0; n < bb.N; n++ {
				data, err := s.MarshalVT()
				require.NoError(bb, err)
				fmt.Println("buf size", len(data))
			}
		})
	}
}

func Benchmark_Unarshall_Array(b *testing.B) {
	for _, keyCount := range ranges {
		runKey := fmt.Sprintf("%d_keys", keyCount)
		b.Run(runKey, func(bb *testing.B) {
			s := &OutputDataTest{
				Items: make([]*CacheItem, keyCount),
			}

			bb.StopTimer()
			for i := 0; i < keyCount; i++ {
				s.Items[i] = &CacheItem{
					BlockNum:  3,
					BlockId:   uuid.NewString(),
					Payload:   []byte(strconv.FormatInt(int64(i), 10)),
					Timestamp: timestamppb.New(time.Now()),
					Cursor:    uuid.NewString(),
				}

			}

			data, err := s.MarshalVT()
			require.NoError(bb, err)

			bb.ResetTimer()
			bb.StartTimer()
			bb.ReportAllocs()

			for n := 0; n < bb.N; n++ {

				o := &OutputDataTest{}
				err := o.UnmarshalVTNoAlloc(data)
				require.NoError(bb, err)

				itemsMap := make(map[string]*CacheItem)

				for _, item := range o.Items {
					itemsMap[item.BlockId] = item
				}
			}
		})
	}
}
