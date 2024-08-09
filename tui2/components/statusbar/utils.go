package statusbar

import "github.com/jhump/protoreflect/dynamic"

func bytesRepr(in dynamic.BytesRepresentation) string {
	switch in {
	case dynamic.BytesAsBase58:
		return "base58"
	case dynamic.BytesAsBase64:
		return "base64"
	case dynamic.BytesAsHex:
		return "hex"
	case dynamic.BytesAsString:
		return "string"
	}
	return "unknown"
}

func showHide(in bool) string {
	if in {
		return "shown"
	}
	return "hidden"
}
