package execout

//func TestExtractClocks(t *testing.T) {
//	cases := []struct {
//		name              string
//		file              *File
//		clocksDistributor map[uint64]*pbsubstreams.Clock
//		expectedResult    map[uint64]*pbsubstreams.Clock
//	}{
//		{
//			name: "sunny path",
//			file: &File{
//				moduleName: "sunny_path",
//				Kv:         map[string]*pboutput.Item{"id1": {BlockNum: 1, BlockId: "1"}, "id2": {BlockNum: 2, BlockId: "3"}},
//			},
//			clocksDistributor: map[uint64]*pbsubstreams.Clock{},
//			expectedResult:    map[uint64]*pbsubstreams.Clock{1: {Number: 1, Id: "1"}, 2: {Number: 2, Id: "3"}},
//		},
//	}
//
//	for _, c := range cases {
//		t.Run(c.name, func(t *testing.T) {
//			c.file.ExtractClocks(c.clocksDistributor)
//			require.Equal(t, c.expectedResult, c.clocksDistributor)
//		})
//	}
//}

//func TestReadWriteOrdered(t *testing.T) {
//	t.Run("write previous KV", func(t *testing.T) {
//		ctx := context.Background()
//		tmp, err := os.MkdirTemp("", "temp_test")
//		defer os.RemoveAll(tmp)
//
//		//		require.NoError(t, err)
//		//		stor, err := dstore.NewStore(tmp, "", "", true)
//		//		require.NoError(t, err)
//		//		file := &File{
//		//			store:      stor,
//		//			ModuleName: "write_previous_kv",
//		//			Kv:         map[string]*pboutput.Item{"id1": {BlockNum: 1, BlockId: "1"}, "id2": {BlockNum: 2, BlockId: "3"}},
//		//			Range: &block.Range{
//		//				StartBlock:        100,
//		//				ExclusiveEndBlock: 200,
//		//			},
//		//			logger: zap.NewNop(),
//		//		}
//		//		file.WriteAsYouGo(ctx)
//		//
//		//		err = file.Save(ctx)
//		//		require.NoError(t, err)
//
//		r, err := stor.OpenObject(ctx, file.Filename())
//		require.NoError(t, err)
//
//		// decode fully
//		data, err := io.ReadAll(r)
//		require.NoError(t, err)
//
//		arr := &pboutput.Array{}
//		err = arr.UnmarshalVTUnsafe(data)
//		require.NoError(t, err)
//
//		assert.True(t, arr.Ordered)
//		assert.Len(t, arr.Items, 2)
//		require.NoError(t, r.Close())
//
//		// decode sequentially
//		r, err = stor.OpenObject(ctx, file.Filename())
//		require.NoError(t, err)
//
//		isOrdered, readBytes, err := streamproto.ReadOrderedBool(r)
//		require.NoError(t, err)
//		require.True(t, isOrdered)
//		require.True(t, bytes.Equal([]byte{0x10, 0x01}, readBytes))
//
//		item, err := streamproto.ReadNextItem(r)
//		require.NoError(t, err)
//		require.Equal(t, item.BlockNum, uint64(1))
//		require.Equal(t, item.BlockId, "1")
//
//		item, err = streamproto.ReadNextItem(r)
//		require.NoError(t, err)
//		require.Equal(t, item.BlockNum, uint64(2))
//		require.Equal(t, item.BlockId, "3")
//
//		item, err = streamproto.ReadNextItem(r)
//		require.NoError(t, err)
//		require.Nil(t, item)
//	})
//}
//
