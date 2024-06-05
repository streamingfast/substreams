// Copyright 2021 dfuse Platform Inc.
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

package tools

import (
	"os"

	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/client"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
)

var Cmd = &cobra.Command{Use: "tools", Short: "Developer tools related to substreams"}

var Example = func(in string) string {
	return string(cli.Example(in))
}

var ExamplePrefixed = func(prefix, in string) string {
	return string(cli.ExamplePrefixed(prefix, in))
}

func ReadAPIToken(cmd *cobra.Command, envFlagName string) string {
	envVar := sflags.MustGetString(cmd, envFlagName)
	value := os.Getenv(envVar)
	if value != "" {
		return value
	}

	return os.Getenv("SF_API_TOKEN")
}

func ReadAPIKey(cmd *cobra.Command, envFlagName string) string {
	envVar := sflags.MustGetString(cmd, envFlagName)
	value := os.Getenv(envVar)
	if value != "" {
		return value
	}

	return os.Getenv("SUBSTREAMS_API_KEY")
}

func GetAuth(cmd *cobra.Command, envFlagApiKey, envFlagJwt string) (authToken string, authType client.AuthType) {

	authType = client.None

	if authToken = ReadAPIKey(cmd, envFlagApiKey); authToken != "" {
		authType = client.ApiKey
		return
	}

	if authToken = ReadAPIToken(cmd, envFlagJwt); authToken != "" {
		authType = client.JWT
		return
	}

	return
}
