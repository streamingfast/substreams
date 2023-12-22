package manifest

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_HashModule(t *testing.T) {
	tests := []struct {
		file   string
		hashes map[string]string
	}{
		{
			file:   "testdata/bare_minimum.yaml",
			hashes: map[string]string{},
		},
		{
			file: "testdata/binaries_relative_path.yaml",
			hashes: map[string]string{
				"test_mapper": "00dec0cb7de44da55d670835234fcd6d9c801fc9",
			},
		},
		{
			file: "testdata/binaries_relative_path.yaml",
			hashes: map[string]string{
				"test_mapper": "00dec0cb7de44da55d670835234fcd6d9c801fc9",
			},
		},
		{
			file: "testdata/univ3-first.yaml",
			hashes: map[string]string{
				"graph_out":                    "6ee6652ef66a55a5f9081fab9e5d0d71bdd9a3af",
				"map_extract_data_types":       "c77f2b113aa378cfcfca763c9d792d450a7b8e59",
				"map_pools_created":            "281a60e619221339a867c45debe00a76f48807ab",
				"map_tokens_whitelist_pools":   "a9a550ccf621ea394c401de108930409ed952d77",
				"store_derived_factory_tvl":    "285cb4af8009d3f74eda50922018dea4ed093eb6",
				"store_derived_tvl":            "9f4fd7ef48cef8e4e1e27821cf4b3c8e47ff9ce4",
				"store_eth_prices":             "37a3314922ffe0200fa227dee43df67d9753bb5c",
				"store_max_windows":            "f00616fd3fe54c72350d45eea499e4fa3b98583e",
				"store_min_windows":            "e924c7a10c688dc70eb86eb10960bc6d5a0c75d6",
				"store_native_amounts":         "3f2d5d0e4ea7611d05c5d78d8b83220fadb8dc67",
				"store_pool_count":             "804bd4401819845e25f84372e2ca7956755a6916",
				"store_pool_liquidities":       "fbd8752e9ed4e21deb4e576e159a9cff8ad768f6",
				"store_pool_sqrt_price":        "0d07ee6b821db6c2174a23ad401e92384e88aa96",
				"store_pools_created":          "65da4dab1cf1556cb649da4d8ba4102a8af0572d",
				"store_positions":              "c727fb136b06f2b436dd967e3a9dd84af63c2a86",
				"store_prices":                 "5c513743460afa2de8f1f2a0b2672e0a4a55c802",
				"store_swaps_volume":           "de327e188adbce08f5f72a8f197124a336878091",
				"store_ticks_liquidities":      "303ee913f3180f7d882d90e686471950dee281d2",
				"store_token_tvl":              "76a5918a733d131eb6d90111ab52d9660220ad88",
				"store_tokens":                 "e6f833dba48a6cbb24d6cf7feb870e2fb58bf6e8",
				"store_tokens_whitelist_pools": "b1cfda6ca52c1189d133b8709872c7877b022150",
				"store_total_tx_counts":        "bbb35b86cf1ecf516dfc57f0c1381deede69df14",
			},
		},

		{
			file: "testdata/univ3-second.yaml",
			hashes: map[string]string{
				"kv_out":                                 "1878855c8b11e211e19b8508047484b1acc6bfe6",
				"uniswapv3:graph_out":                    "6ee6652ef66a55a5f9081fab9e5d0d71bdd9a3af",
				"uniswapv3:map_extract_data_types":       "c77f2b113aa378cfcfca763c9d792d450a7b8e59",
				"uniswapv3:map_pools_created":            "281a60e619221339a867c45debe00a76f48807ab",
				"uniswapv3:map_tokens_whitelist_pools":   "a9a550ccf621ea394c401de108930409ed952d77",
				"uniswapv3:store_derived_factory_tvl":    "285cb4af8009d3f74eda50922018dea4ed093eb6",
				"uniswapv3:store_derived_tvl":            "9f4fd7ef48cef8e4e1e27821cf4b3c8e47ff9ce4",
				"uniswapv3:store_eth_prices":             "37a3314922ffe0200fa227dee43df67d9753bb5c",
				"uniswapv3:store_max_windows":            "f00616fd3fe54c72350d45eea499e4fa3b98583e",
				"uniswapv3:store_min_windows":            "e924c7a10c688dc70eb86eb10960bc6d5a0c75d6",
				"uniswapv3:store_native_amounts":         "3f2d5d0e4ea7611d05c5d78d8b83220fadb8dc67",
				"uniswapv3:store_pool_count":             "804bd4401819845e25f84372e2ca7956755a6916",
				"uniswapv3:store_pool_liquidities":       "fbd8752e9ed4e21deb4e576e159a9cff8ad768f6",
				"uniswapv3:store_pool_sqrt_price":        "0d07ee6b821db6c2174a23ad401e92384e88aa96",
				"uniswapv3:store_pools_created":          "65da4dab1cf1556cb649da4d8ba4102a8af0572d",
				"uniswapv3:store_positions":              "c727fb136b06f2b436dd967e3a9dd84af63c2a86",
				"uniswapv3:store_prices":                 "5c513743460afa2de8f1f2a0b2672e0a4a55c802",
				"uniswapv3:store_swaps_volume":           "de327e188adbce08f5f72a8f197124a336878091",
				"uniswapv3:store_ticks_liquidities":      "303ee913f3180f7d882d90e686471950dee281d2",
				"uniswapv3:store_token_tvl":              "76a5918a733d131eb6d90111ab52d9660220ad88",
				"uniswapv3:store_tokens":                 "e6f833dba48a6cbb24d6cf7feb870e2fb58bf6e8",
				"uniswapv3:store_tokens_whitelist_pools": "b1cfda6ca52c1189d133b8709872c7877b022150",
				"uniswapv3:store_total_tx_counts":        "bbb35b86cf1ecf516dfc57f0c1381deede69df14",
			},
		},
		{
			file: "testdata/with-params.yaml",
			hashes: map[string]string{
				"mod1": "a9f22492be1fb13050c07f1502d5a6e78577dd80",
				"mod2": "6aca30692dfa835efe09fbf51b0a1735ea3b3155",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			reader, err := NewReader(test.file)
			require.NoError(t, err)

			manifest, graph, err := reader.Read()
			require.NoError(t, err)

			hashes := NewModuleHashes()
			compare := map[string]string{}
			for _, mod := range graph.modules {
				hash, err := hashes.HashModule(manifest.Modules, mod, graph)
				require.NoError(t, err)
				compare[mod.Name] = hex.EncodeToString(hash)
			}
			assert.Equal(t, test.hashes, compare)
		})
	}
}
