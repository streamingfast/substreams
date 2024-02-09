package client

type Headers map[string]string

const ApiKeyHeader = "x-api-key"

func (h Headers) Append(headers map[string]string) map[string]string {
	for key, value := range headers {
		h[key] = value
	}
	return h
}

func (h Headers) ToArray() []string {
	res := make([]string, 0, len(h)*2)
	for key, value := range h {
		res = append(res, key, value)
	}
	return res
}

func (h Headers) IsSet() bool {
	return len(h) > 0
}
