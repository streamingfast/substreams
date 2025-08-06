package sink

import (
	"os"

	"github.com/streamingfast/substreams/client"
)

type authenticator struct {
	apiKeyEnvVar   string
	apiTokenEnvVar string
}

func newAuthenticator(apiKeyEnvVar string, apiTokenEnvVar string) *authenticator {
	return &authenticator{
		apiKeyEnvVar:   apiKeyEnvVar,
		apiTokenEnvVar: apiTokenEnvVar,
	}
}

func (a *authenticator) GetApiKey() string {
	return a.apiKeyEnvVar
}

func (a *authenticator) GetApiToken() string {
	return a.apiTokenEnvVar
}

func (a *authenticator) GetTokenAndType() (authToken string, authType client.AuthType) {
	apiKeyFromEnv := os.Getenv(a.apiKeyEnvVar)
	if apiKeyFromEnv != "" {
		return apiKeyFromEnv, client.ApiKey
	}

	apiTokenFromEnv := os.Getenv(a.apiTokenEnvVar)
	if apiTokenFromEnv != "" {
		return apiTokenFromEnv, client.JWT
	}
	return "", client.None
}
