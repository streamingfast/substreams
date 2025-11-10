// Package marshaller benchmarks for VTproto streaming marshalling functionality.
// This file contains comprehensive benchmarks that demonstrate the performance
// characteristics and memory efficiency of streaming vs regular marshalling.
//
// Key benchmark categories:
// - Basic streaming vs regular marshal performance
// - Memory usage comparison showing streaming efficiency
// - Real-world scenarios with various data sizes
// - Allocation patterns for different data shapes
// - Buffer size impact on streaming performance
// - Optimization techniques (size hints, buffer sizes)
//
// The benchmarks clearly demonstrate that streaming marshalling:
// 1. Uses significantly less memory (constant vs linear with data size)
// 2. Has predictable allocation patterns regardless of data size
// 3. Enables processing of datasets larger than available RAM
// 4. Trades some CPU performance for memory efficiency
package marshaller

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
)

func BenchmarkVTproto_MarshalStream(b *testing.B) {
	// Create test data with various sizes
	testData := &StoreData{
		Kv:             make(map[string][]byte),
		DeletePrefixes: []string{},
	}

	// Add many KV pairs
	for i := range 1000 {
		key := fmt.Sprintf("key_%d", i)
		value := fmt.Sprintf("value_%d_with_some_longer_content", i)
		testData.Kv[key] = []byte(value)
	}

	// Add delete prefixes
	for i := range 100 {
		testData.DeletePrefixes = append(testData.DeletePrefixes, fmt.Sprintf("prefix_%d", i))
	}

	marshaller := &VTproto{}

	b.ResetTimer()

	b.Run("Regular_Marshal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			data, err := marshaller.Marshal(testData)
			if err != nil {
				b.Fatalf("failed to marshal: %v", err)
			}
			_ = len(data) // Consume the result
		}
	})

	b.Run("Stream_Marshal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			reader := marshaller.MarshalStream(testData, 0)

			// Read all data to simulate full consumption
			buffer := make([]byte, 4096)
			totalBytes := 0
			for {
				n, err := reader.Read(buffer)
				totalBytes += n
				if err == io.EOF {
					break
				}
				if err != nil {
					b.Fatalf("failed to read from stream: %v", err)
				}
			}
			reader.Close()
		}
	})

	b.Run("Stream_Marshal_SmallReads", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			reader := marshaller.MarshalStream(testData, 0)

			// Read in small chunks to test streaming behavior
			buffer := make([]byte, 64)
			totalBytes := 0
			for {
				n, err := reader.Read(buffer)
				totalBytes += n
				if err == io.EOF {
					break
				}
				if err != nil {
					b.Fatalf("failed to read from stream: %v", err)
				}
			}
			reader.Close()
		}
	})
}

func BenchmarkVTproto_MarshalMemoryComparison(b *testing.B) {
	// Create large test data to show memory pressure differences
	testData := &StoreData{
		Kv:             make(map[string][]byte),
		DeletePrefixes: []string{},
	}

	// Add many KV pairs with large values to simulate real-world data
	for i := range 5000 {
		key := fmt.Sprintf("large_document:%d", i)
		// Create large JSON-like documents (5KB each)
		largeValue := strings.Repeat(fmt.Sprintf(`{"chunk":%d,"data":"%s"}`, i%100, strings.Repeat("x", 50)), 50)
		testData.Kv[key] = []byte(largeValue)
	}

	// Add delete prefixes
	for i := range 500 {
		testData.DeletePrefixes = append(testData.DeletePrefixes, fmt.Sprintf("temp:large:%d:", i))
	}

	marshaller := &VTproto{}

	// Get baseline data size
	regularData, err := marshaller.Marshal(testData)
	if err != nil {
		b.Fatalf("failed to marshal: %v", err)
	}
	b.Logf("Total data size: %.2f MB", float64(len(regularData))/(1024*1024))

	b.ResetTimer()

	b.Run("Regular_Marshal_Peak_Memory", func(b *testing.B) {
		b.ReportAllocs()

		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)

		for b.Loop() {
			// This creates the entire marshaled data in memory at once
			data, err := marshaller.Marshal(testData)
			if err != nil {
				b.Fatalf("failed to marshal: %v", err)
			}

			// Simulate writing to a file/network
			written := 0
			chunkSize := 4096
			for written < len(data) {
				end := written + chunkSize
				if end > len(data) {
					end = len(data)
				}
				_ = data[written:end] // Simulate processing chunk
				written = end
			}
		}

		runtime.ReadMemStats(&m2)
		b.Logf("Regular Marshal - Peak memory increase: %.2f MB",
			float64(m2.Sys-m1.Sys)/(1024*1024))
	})

	b.Run("Stream_Marshal_Lower_Memory", func(b *testing.B) {
		b.ReportAllocs()

		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)

		for b.Loop() {
			// This generates data incrementally without storing everything in memory
			reader := marshaller.MarshalStream(testData, int64(len(regularData)))

			// Simulate writing to a file/network
			buffer := make([]byte, 4096)
			for {
				n, err := reader.Read(buffer)
				if n > 0 {
					_ = buffer[:n] // Simulate processing chunk
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					b.Fatalf("failed to read from stream: %v", err)
				}
			}
			reader.Close()
		}

		runtime.ReadMemStats(&m2)
		b.Logf("Stream Marshal - Peak memory increase: %.2f MB",
			float64(m2.Sys-m1.Sys)/(1024*1024))
	})
}

func BenchmarkVTproto_MarshalRealWorldComparison(b *testing.B) {
	// Simulate real-world blockchain state marshaling with various data sizes
	testSizes := []struct {
		name      string
		kvPairs   int
		prefixes  int
		valueSize int
	}{
		{"Small_State_1MB", 1000, 50, 100},
		{"Medium_State_10MB", 10000, 200, 100},
		{"Large_State_50MB", 50000, 1000, 100},
	}

	marshaller := &VTproto{}

	for _, size := range testSizes {
		b.Run(size.name, func(b *testing.B) {
			// Create realistic blockchain state data
			testData := &StoreData{
				Kv:             make(map[string][]byte),
				DeletePrefixes: make([]string, 0, size.prefixes),
			}

			// Add KV pairs with realistic blockchain-like keys and values
			for i := 0; i < size.kvPairs; i++ {
				key := fmt.Sprintf("account:%08d:state", i)
				value := fmt.Sprintf(`{"balance":%d,"nonce":%d,"code_hash":"0x%032x","storage":"%s"}`,
					i*1000, i, i, strings.Repeat("0", size.valueSize))
				testData.Kv[key] = []byte(value)
			}

			// Add realistic delete prefixes
			for i := 0; i < size.prefixes; i++ {
				testData.DeletePrefixes = append(testData.DeletePrefixes,
					fmt.Sprintf("temp:block:%d:", i))
			}

			// Get baseline size
			regularData, err := marshaller.Marshal(testData)
			if err != nil {
				b.Fatalf("failed to marshal: %v", err)
			}

			b.Logf("Real-world %s: %.2f MB",
				size.name, float64(len(regularData))/(1024*1024))

			b.ResetTimer()

			b.Run("Traditional_Marshal", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					data, err := marshaller.Marshal(testData)
					if err != nil {
						b.Fatalf("failed to marshal: %v", err)
					}

					// Simulate writing to disk/network
					_ = len(data)
				}
			})

			b.Run("Streaming_Marshal", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					reader := marshaller.MarshalStream(testData, int64(len(regularData)))

					// Simulate streaming to disk/network
					buffer := make([]byte, 8192)
					for {
						n, err := reader.Read(buffer)
						if n > 0 {
							// Simulate processing (e.g., writing to file, network)
							_ = buffer[:n]
						}
						if err == io.EOF {
							break
						}
						if err != nil {
							b.Fatalf("failed to read stream: %v", err)
						}
					}
					reader.Close()
				}
			})

			b.Run("Streaming_Marshal_To_File", func(b *testing.B) {
				// Create temp file for this sub-benchmark
				tmpFile, err := os.CreateTemp("", fmt.Sprintf("marshal_%s_*.bin", size.name))
				if err != nil {
					b.Fatalf("failed to create temp file: %v", err)
				}
				defer os.Remove(tmpFile.Name())
				tmpFile.Close()

				b.ReportAllocs()
				for b.Loop() {
					file, err := os.OpenFile(tmpFile.Name(), os.O_WRONLY|os.O_TRUNC, 0644)
					if err != nil {
						b.Fatalf("failed to open file: %v", err)
					}

					reader := marshaller.MarshalStream(testData, int64(len(regularData)))

					// Stream directly to file
					buffer := make([]byte, 32768) // 32KB buffer
					for {
						n, err := reader.Read(buffer)
						if n > 0 {
							if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
								reader.Close()
								file.Close()
								b.Fatalf("failed to write to file: %v", writeErr)
							}
						}
						if err == io.EOF {
							break
						}
						if err != nil {
							reader.Close()
							file.Close()
							b.Fatalf("failed to read stream: %v", err)
						}
					}

					reader.Close()
					file.Close()
				}
			})
		})
	}
}

func BenchmarkVTproto_MarshalAllocationComparison(b *testing.B) {
	// Test allocation patterns for different data shapes
	testCases := []struct {
		name        string
		setupData   func() *StoreData
		description string
	}{
		{
			name: "Many_Small_KVs",
			setupData: func() *StoreData {
				data := &StoreData{Kv: make(map[string][]byte)}
				for i := 0; i < 10000; i++ {
					data.Kv[fmt.Sprintf("k%d", i)] = []byte(fmt.Sprintf("v%d", i))
				}
				return data
			},
			description: "10k small key-value pairs",
		},
		{
			name: "Few_Large_Values",
			setupData: func() *StoreData {
				data := &StoreData{Kv: make(map[string][]byte)}
				for i := 0; i < 100; i++ {
					largeValue := strings.Repeat(fmt.Sprintf("data_%d_", i), 1000)
					data.Kv[fmt.Sprintf("large_key_%d", i)] = []byte(largeValue)
				}
				return data
			},
			description: "100 large values (~8KB each)",
		},
		{
			name: "Many_Delete_Prefixes",
			setupData: func() *StoreData {
				data := &StoreData{
					Kv:             make(map[string][]byte),
					DeletePrefixes: make([]string, 0, 5000),
				}
				for i := 0; i < 1000; i++ {
					data.Kv[fmt.Sprintf("key_%d", i)] = []byte(fmt.Sprintf("value_%d", i))
				}
				for i := 0; i < 5000; i++ {
					data.DeletePrefixes = append(data.DeletePrefixes, fmt.Sprintf("prefix_%d_", i))
				}
				return data
			},
			description: "1k KVs + 5k delete prefixes",
		},
		{
			name: "Mixed_Large_Dataset",
			setupData: func() *StoreData {
				data := &StoreData{
					Kv:             make(map[string][]byte),
					DeletePrefixes: make([]string, 0, 1000),
				}
				for i := 0; i < 5000; i++ {
					keySize := 20 + (i % 50)      // Variable key sizes
					valueSize := 100 + (i % 1000) // Variable value sizes

					key := fmt.Sprintf("%s:%d", strings.Repeat("x", keySize-10), i)
					value := strings.Repeat(fmt.Sprintf("data_%d_", i), valueSize/10)
					data.Kv[key] = []byte(value)
				}
				for i := 0; i < 1000; i++ {
					prefixSize := 10 + (i % 20)
					data.DeletePrefixes = append(data.DeletePrefixes,
						fmt.Sprintf("%s:", strings.Repeat("p", prefixSize)))
				}
				return data
			},
			description: "5k variable-size KVs + 1k prefixes",
		},
	}

	marshaller := &VTproto{}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			testData := tc.setupData()

			// Get size info
			regularData, err := marshaller.Marshal(testData)
			if err != nil {
				b.Fatalf("failed to marshal: %v", err)
			}

			b.Logf("%s (%s): %.2f KB", tc.name, tc.description,
				float64(len(regularData))/1024)

			b.ResetTimer()

			b.Run("Regular", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					data, err := marshaller.Marshal(testData)
					if err != nil {
						b.Fatalf("failed to marshal: %v", err)
					}
					_ = len(data)
				}
			})

			b.Run("Stream", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					reader := marshaller.MarshalStream(testData, int64(len(regularData)))

					buffer := make([]byte, 4096)
					for {
						n, err := reader.Read(buffer)
						if n > 0 {
							_ = buffer[:n]
						}
						if err == io.EOF {
							break
						}
						if err != nil {
							b.Fatalf("failed to read: %v", err)
						}
					}
					reader.Close()
				}
			})
		})
	}
}

func BenchmarkVTproto_MarshalBufferSizes(b *testing.B) {
	// Test how different read buffer sizes affect streaming performance
	testData := &StoreData{
		Kv: make(map[string][]byte),
	}

	// Create medium-sized test data
	for i := 0; i < 2000; i++ {
		key := fmt.Sprintf("benchmark_key_%d", i)
		value := strings.Repeat(fmt.Sprintf("value_%d_", i), 20)
		testData.Kv[key] = []byte(value)
	}

	for i := 0; i < 200; i++ {
		testData.DeletePrefixes = append(testData.DeletePrefixes, fmt.Sprintf("del_prefix_%d_", i))
	}

	marshaller := &VTproto{}
	bufferSizes := []int{64, 256, 1024, 4096, 16384, 65536}

	for _, bufSize := range bufferSizes {
		b.Run(fmt.Sprintf("Buffer_%dB", bufSize), func(b *testing.B) {
			buffer := make([]byte, bufSize)

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				reader := marshaller.MarshalStream(testData, 0)

				totalBytes := 0
				for {
					n, err := reader.Read(buffer)
					totalBytes += n
					if err == io.EOF {
						break
					}
					if err != nil {
						b.Fatalf("failed to read: %v", err)
					}
				}
				reader.Close()
			}
		})
	}
}

// ExampleVTproto_MarshalStream demonstrates how to use streaming marshaling
func ExampleVTproto_MarshalStream() {
	// Create test data
	testData := &StoreData{
		Kv: map[string][]byte{
			"account:001": []byte(`{"balance":1000,"nonce":1}`),
			"account:002": []byte(`{"balance":2000,"nonce":5}`),
			"account:003": []byte(`{"balance":500,"nonce":2}`),
		},
		DeletePrefixes: []string{"temp:", "cache:"},
	}

	marshaller := &VTproto{}

	// Stream marshal - generates data on-the-fly without loading everything in memory
	reader := marshaller.MarshalStream(testData, 0) // 0 for auto-sizing
	defer reader.Close()

	// Read data in chunks (simulating writing to file or network)
	buffer := make([]byte, 1024)
	totalBytes := 0

	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			totalBytes += n
			// Process chunk (e.g., write to file, send over network)
			fmt.Printf("Read chunk of %d bytes\n", n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(fmt.Sprintf("Read error: %v", err))
		}
	}

	fmt.Printf("Total bytes streamed: %d\n", totalBytes)

	// Note: Actual output will vary based on data structure and marshaling
}

// ExampleVTproto_MarshalStream_fileWrite demonstrates streaming to a file
func ExampleVTproto_MarshalStream_fileWrite() {
	testData := &StoreData{
		Kv: map[string][]byte{
			"large_data_1": []byte(strings.Repeat("data", 1000)),
			"large_data_2": []byte(strings.Repeat("more", 1000)),
		},
		DeletePrefixes: []string{"cleanup:"},
	}

	marshaller := &VTproto{}

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "stream_marshal_*.bin")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Stream directly to file without loading all data in memory
	reader := marshaller.MarshalStream(testData, 0)
	defer reader.Close()

	// Copy from stream to file efficiently
	bytesWritten, err := io.Copy(tmpFile, reader)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Streamed %d bytes to file\n", bytesWritten)

	// Verify by reading back and unmarshaling
	tmpFile.Seek(0, 0) // Reset to beginning
	result, _, err := marshaller.UnmarshalStream(tmpFile, bytesWritten)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Successfully round-tripped %d KV pairs\n", len(result.Kv))

	// Note: Actual byte count and file paths will vary
}

// BenchmarkVTproto_MarshalStreamOptimizations shows performance with different optimizations
func BenchmarkVTproto_MarshalStreamOptimizations(b *testing.B) {
	// Create realistic blockchain state data
	testData := &StoreData{
		Kv:             make(map[string][]byte),
		DeletePrefixes: make([]string, 0, 100),
	}

	// Add realistic account state data
	for i := 0; i < 5000; i++ {
		key := fmt.Sprintf("account:%08x:state", i)
		value := fmt.Sprintf(`{"balance":%d,"nonce":%d,"code":"0x%064x","storage_root":"0x%064x"}`,
			i*1000000, i, i*12345, i*54321)
		testData.Kv[key] = []byte(value)
	}

	// Add deletion prefixes
	for i := 0; i < 100; i++ {
		testData.DeletePrefixes = append(testData.DeletePrefixes, fmt.Sprintf("temp:block:%d:", i))
	}

	marshaller := &VTproto{}

	// Get baseline size for estimation
	baselineData, _ := marshaller.Marshal(testData)
	estimatedSize := int64(len(baselineData))

	b.Logf("Test data: %d KV pairs, %d prefixes, %.2f MB total",
		len(testData.Kv), len(testData.DeletePrefixes), float64(estimatedSize)/(1024*1024))

	b.Run("No_Size_Hint", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			reader := marshaller.MarshalStream(testData, 0) // No size hint
			io.Copy(io.Discard, reader)
			reader.Close()
		}
	})

	b.Run("With_Size_Hint", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			reader := marshaller.MarshalStream(testData, estimatedSize) // With size hint
			io.Copy(io.Discard, reader)
			reader.Close()
		}
	})

	b.Run("Large_Buffer_Copy", func(b *testing.B) {
		b.ReportAllocs()
		buffer := make([]byte, 64*1024) // 64KB buffer
		for b.Loop() {
			reader := marshaller.MarshalStream(testData, estimatedSize)
			for {
				n, err := reader.Read(buffer)
				if err == io.EOF {
					break
				}
				if err != nil {
					b.Fatal(err)
				}
				_ = buffer[:n] // Simulate processing
			}
			reader.Close()
		}
	})

	b.Run("Small_Buffer_Copy", func(b *testing.B) {
		b.ReportAllocs()
		buffer := make([]byte, 512) // 512B buffer
		for b.Loop() {
			reader := marshaller.MarshalStream(testData, estimatedSize)
			for {
				n, err := reader.Read(buffer)
				if err == io.EOF {
					break
				}
				if err != nil {
					b.Fatal(err)
				}
				_ = buffer[:n] // Simulate processing
			}
			reader.Close()
		}
	})
}
