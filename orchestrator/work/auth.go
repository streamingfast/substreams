package work

import (
	"context"

	"github.com/streamingfast/dauth"
	"google.golang.org/grpc/metadata"
)

func getAuthDetails(ctx context.Context) (userID string, apiKeyId string, ip string) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return
	}

	userID = getHeader(dauth.SFHeaderUserID, md)
	apiKeyId = getHeader(dauth.SFHeaderApiKeyID, md)
	ip = getHeader(dauth.SFHeaderIP, md)
	return
}

func getHeader(key string, md metadata.MD) string {
	if len(md.Get(key)) > 0 {
		return md.Get(key)[0]
	}
	return ""
}
