package pboutputcache

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// var ranges = []int{10_000, 100_000, 1_000_000, 10_000_000}
var ranges = []int{10_000_000}

// not used, other method is faster, output is incompatible
//func Benchmark_Marshall_Map(b *testing.B) {
//	for _, keyCount := range ranges {
//		runKey := fmt.Sprintf("%d_keys", keyCount)
//		b.Run(runKey, func(bb *testing.B) {
//			s := &Map{
//				Kv: make(map[string]*Item),
//			}
//
//			bb.StopTimer()
//			for i := 0; i < keyCount; i++ {
//				s.Kv[uuid.NewString()] = &Item{
//					BlockNum:  3,
//					BlockId:   uuid.NewString(),
//					Payload:   []byte(strconv.FormatInt(int64(i), 10)),
//					Timestamp: timestamppb.New(time.Now()),
//					Cursor:    uuid.NewString(),
//				}
//			}
//
//			bb.ResetTimer()
//			bb.StartTimer()
//			bb.ReportAllocs()
//
//			for n := 0; n < bb.N; n++ {
//				data, err := s.MarshalVT()
//				require.NoError(bb, err)
//				fmt.Println("buf size", len(data))
//			}
//		})
//	}
//}

func Benchmark_Marshall_Array(b *testing.B) {
	for _, keyCount := range ranges {
		runKey := fmt.Sprintf("%d_keys", keyCount)
		b.Run(runKey, func(bb *testing.B) {
			itemMap := &Map{
				Kv: make(map[string]*Item),
			}

			bb.StopTimer()
			for i := 0; i < keyCount; i++ {
				itemMap.Kv[uuid.NewString()] = &Item{
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
				data, err := itemMap.MarshalFast()
				require.NoError(bb, err)
				fmt.Println("buf size", len(data))
			}
		})
	}
}

func Benchmark_Unmarshall_Array(b *testing.B) {
	for _, keyCount := range ranges {
		runKey := fmt.Sprintf("%d_keys", keyCount)
		b.Run(runKey, func(bb *testing.B) {
			s := &Array{
				Items: make([]*Item, keyCount),
			}

			bb.StopTimer()
			for i := 0; i < keyCount; i++ {
				s.Items[i] = &Item{
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

				o := &Array{}
				err := o.UnmarshalVTNoAlloc(data)
				require.NoError(bb, err)

				itemsMap := make(map[string]*Item)

				for _, item := range o.Items {
					itemsMap[item.BlockId] = item
				}
			}
		})
	}
}

func Test_Unmarshall_Array(t *testing.T) {
	cacheLen := 10
	s := &Array{
		Items: make([]*Item, 10),
	}

	for i := 0; i < cacheLen; i++ {
		s.Items[i] = &Item{
			BlockNum:  3,
			BlockId:   uuid.NewString(),
			Payload:   []byte(strconv.FormatInt(int64(i), 10)),
			Timestamp: timestamppb.New(time.Now()),
			Cursor:    uuid.NewString(),
		}

	}

	data, err := s.MarshalVT()
	require.NoError(t, err)

	o := &Array{}
	err = o.UnmarshalVTNoAlloc(data)
	require.NoError(t, err)

	itemsMap := make(map[string]*Item)

	for _, item := range o.Items {
		itemsMap[item.BlockId] = item
	}

	// comparing output's itemMap with original 's'
	for _, item := range s.Items {
		assert.Equal(t, item.Cursor, itemsMap[item.BlockId].Cursor)
	}
}
