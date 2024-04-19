package server

import (
	"go.uber.org/zap"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/cors"
)

func (s *server) corsOption() *cors.Cors {
	return cors.New(cors.Options{
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowOriginFunc: s.allowedOrigin,
		AllowedHeaders:  []string{"*"},
		ExposedHeaders: []string{
			// Content-Type is in the default safelist.
			"Accept",
			"Accept-Encoding",
			"Accept-Post",
			"Connect-Accept-Encoding",
			"Connect-Content-Encoding",
			"Content-Encoding",
			"Grpc-Accept-Encoding",
			"Grpc-Encoding",
			"Grpc-Message",
			"Grpc-State",
			"Grpc-State-Details-Bin",
		},
		MaxAge: int(2 * time.Hour / time.Second),
	})
}

func (s *server) allowedOrigin(origin string) bool {
	s.logger.Debug("allowed origin", zap.String("origin", origin))

	if s.corsHostRegexAllow == nil {
		s.logger.Warn("allowed origin, no host regex allowed filter specify denying origin", zap.String("origin", origin))
		return false
	}

	uri, err := url.Parse(origin)
	if err != nil {
		s.logger.Warn("failed to parse origin", zap.String("origin", origin), zap.Error(err))
		return false
	}
	return s.corsHostRegexAllow.MatchString(uri.Host)
}
