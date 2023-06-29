package main

import (
	"log"
	"strings"
)

// util to parse headers flags
func parseHeaders(headers []string) map[string]string {
	if headers == nil {
		return nil
	}
	result := make(map[string]string)
	for _, header := range headers {
		parts := strings.Split(header, ":")
		if len(parts) != 2 {
			log.Fatalf("invalid header format: %s", header)
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result
}
