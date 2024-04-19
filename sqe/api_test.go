// Copyright 2024 StreamingFast Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqe

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// func TestExpressionToBleveQuery(t *testing.T) {
// 	tests := []struct {
// 		in          string
// 		expectBleve string
// 	}{
// 		{
// 			in:          "account:eoscanadacom",
// 			expectBleve: `{"term":"eoscanadacom","field":"account"}`,
// 		},
// 		{
// 			in:          "data.active:true",
// 			expectBleve: `{"bool":true,"field":"data.active"}`,
// 		},
// 		{
// 			in:          "data.active:false",
// 			expectBleve: `{"bool":false,"field":"data.active"}`,
// 		},
// 		{
// 			in:          `data.active:"true"`,
// 			expectBleve: `{"term":"true","field":"data.active"}`,
// 		},
// 		{
// 			in:          "receiver:eoscanadacom account:eoscanadacom",
// 			expectBleve: `{"conjuncts":[{"term":"eoscanadacom","field":"receiver"},{"term":"eoscanadacom","field":"account"}]}`,
// 		},
// 		{
// 			in:          "account:eoscanadacom receiver:eoscanadacom",
// 			expectBleve: `{"conjuncts":[{"term":"eoscanadacom","field":"account"},{"term":"eoscanadacom","field":"receiver"}]}`,
// 		},
// 		{
// 			in:          "receiver:eoscanadacom (action:transfer OR action:issue)",
// 			expectBleve: `{"conjuncts":[{"term":"eoscanadacom","field":"receiver"},{"disjuncts":[{"term":"transfer","field":"action"},{"term":"issue","field":"action"}],"min":1}]}`,
// 		},
// 		{
// 			in:          "receiver:eoscanadacom -(action:transfer OR action:issue)",
// 			expectBleve: `{"conjuncts":[{"term":"eoscanadacom","field":"receiver"},{"must_not":{"disjuncts":[{"disjuncts":[{"term":"transfer","field":"action"},{"term":"issue","field":"action"}],"min":1}],"min":0}}]}`,
// 		},
// 		{
// 			in:          "-receiver:eoscanadacom (action:transfer OR action:issue)",
// 			expectBleve: `{"conjuncts":[{"must_not":{"disjuncts":[{"term":"eoscanadacom","field":"receiver"}],"min":0}},{"disjuncts":[{"term":"transfer","field":"action"},{"term":"issue","field":"action"}],"min":1}]}`,
// 		},
// 		{
// 			in:          "-action:patate",
// 			expectBleve: `{"must_not":{"disjuncts":[{"term":"patate","field":"action"}],"min":0}}`,
// 		},
// 		{
// 			in: "receiver:eoscanadacom (action:transfer OR action:issue) account:eoscanadacom (data.from:eoscanadacom OR data.to:eoscanadacom)",
// 			expectBleve: `{
// 				"conjuncts": [
// 				  { "term": "eoscanadacom", "field": "receiver" },
// 				  { "disjuncts": [
// 					  { "term": "transfer", "field": "action" },
// 					  { "term": "issue", "field": "action" }
// 					], "min": 1
// 				  },
// 				  { "term": "eoscanadacom", "field": "account" },
// 				  { "disjuncts": [
// 					  { "term": "eoscanadacom", "field": "data.from" },
// 					  { "term": "eoscanadacom", "field": "data.to" }
// 					], "min": 1
// 				  }
// 				]
// 			  }`,
// 		},
// 	}

// 	for idx, test := range tests {
// 		t.Run(fmt.Sprintf("index %d", idx+1), func(t *testing.T) {
// 			ast, err := Parse(context.Background(), test.in)
// 			require.NoError(t, err)

// 			res := ExpressionToBleve(ast)

// 			cnt, err := json.Marshal(res)
// 			require.NoError(t, err)
// 			assert.JSONEq(t, test.expectBleve, string(cnt), "Failed on SQE %q, got %s", test.in, string(cnt))
// 		})
// 	}
// }

func TestExtractAllKeys(t *testing.T) {
	tests := []struct {
		in           string
		expectedKeys []string
	}{
		{
			"account",
			[]string{"account"},
		},
		{
			"data.active",
			[]string{"data.active"},
		},
		{
			"data.active",
			[]string{"data.active"},
		},
		{
			`"data.active"`,
			[]string{"data.active"},
		},
		{
			"receiver account",
			[]string{"receiver", "account"},
		},
		{
			"receiver (action || action)",
			[]string{"receiver", "action"},
		},
		{
			"receiver (action || action) account (data.from || data.to)",
			[]string{"receiver", "action", "account", "data.from", "data.to"},
		},
	}

	for idx, test := range tests {
		t.Run(fmt.Sprintf("index %d", idx+1), func(t *testing.T) {
			ast, err := Parse(context.Background(), test.in)
			require.NoError(t, err)

			actuals := ExtractAllKeys(ast)
			assert.ElementsMatch(t, test.expectedKeys, actuals, "Mistmatch for SQE %q", test.in)
		})
	}
}
