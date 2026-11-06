package pubsub

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

// legacyPublishType is the module output of substreams-sink-pubsub.
// A module of this type is published in that sink's wire format. Every other
// module keeps the webhook JSON.
const legacyPublishType = "sf.substreams.sink.pubsub.v1.Publish"

func isLegacyPublishType(typeURL string) bool {
	for {
		switch {
		case strings.HasPrefix(typeURL, "type.googleapis.com/"):
			typeURL = strings.TrimPrefix(typeURL, "type.googleapis.com/")
		case strings.HasPrefix(typeURL, "proto:"):
			typeURL = strings.TrimPrefix(typeURL, "proto:")
		default:
			return typeURL == legacyPublishType
		}
	}
}

type legacyMessage struct {
	data       []byte
	attributes map[string]string
}

// legacyBlockMessages reproduces substreams-sink-pubsub: one Pub/Sub message
// per Publish.Message, the module's attributes kept, Cursor set to the sink
// cursor, and the ordering key "<9-digit block>_<5-digit index>".
func legacyBlockMessages(blockNum uint64, cursor string, raw []byte) ([]Message, error) {
	parsed, err := parsePublish(raw)
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(parsed))
	for i, msg := range parsed {
		attributes := make(map[string]string, len(msg.attributes)+1)
		for key, value := range msg.attributes {
			attributes[key] = value
		}
		attributes["Cursor"] = cursor
		out = append(out, Message{
			Data:        msg.data,
			Attributes:  attributes,
			OrderingKey: fmt.Sprintf("%09d_%05d", blockNum, i),
		})
	}
	return out, nil
}

// legacyUndoMessage reproduces the substreams-sink-pubsub undo message.
func legacyUndoMessage(lastValidBlock uint64, cursor string) Message {
	return Message{
		Attributes: map[string]string{
			"LastValidBlock": strconv.FormatUint(lastValidBlock, 10),
			"Step":           "Undo",
			"Cursor":         cursor,
		},
	}
}

// parsePublish reads sf.substreams.sink.pubsub.v1.Publish.
// Publish.messages is field 1. Unknown fields are skipped.
func parsePublish(b []byte) ([]legacyMessage, error) {
	var out []legacyMessage
	err := walkProto(b, func(num protowire.Number, typ protowire.Type, raw []byte) error {
		if num != 1 || typ != protowire.BytesType {
			return nil
		}
		body, n := protowire.ConsumeBytes(raw)
		if n < 0 {
			return protowire.ParseError(n)
		}
		msg, err := parseLegacyMessage(body)
		if err != nil {
			return err
		}
		out = append(out, msg)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading Publish output: %w", err)
	}
	return out, nil
}

// parseLegacyMessage reads Publish.Message. data is field 1, attributes is field 2.
func parseLegacyMessage(b []byte) (legacyMessage, error) {
	msg := legacyMessage{attributes: map[string]string{}}
	var sawData bool
	err := walkProto(b, func(num protowire.Number, typ protowire.Type, raw []byte) error {
		switch {
		case num == 1 && typ == protowire.BytesType:
			data, n := protowire.ConsumeBytes(raw)
			if n < 0 {
				return protowire.ParseError(n)
			}
			// Clone keeps a present empty value distinct from an absent one.
			msg.data = bytes.Clone(data)
			sawData = true
		case num == 2 && typ == protowire.BytesType:
			body, n := protowire.ConsumeBytes(raw)
			if n < 0 {
				return protowire.ParseError(n)
			}
			key, value, err := parseLegacyAttribute(body)
			if err != nil {
				return err
			}
			msg.attributes[key] = value
		}
		return nil
	})
	if err != nil {
		return legacyMessage{}, err
	}
	if !sawData {
		msg.data = nil
	}
	return msg, nil
}

// parseLegacyAttribute reads Publish.Attribute. key is field 1, value is field 2.
func parseLegacyAttribute(b []byte) (key, value string, err error) {
	err = walkProto(b, func(num protowire.Number, typ protowire.Type, raw []byte) error {
		if typ != protowire.BytesType || (num != 1 && num != 2) {
			return nil
		}
		s, n := protowire.ConsumeString(raw)
		if n < 0 {
			return protowire.ParseError(n)
		}
		if num == 1 {
			key = s
		} else {
			value = s
		}
		return nil
	})
	return key, value, err
}

func walkProto(b []byte, fn func(num protowire.Number, typ protowire.Type, raw []byte) error) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]
		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		if err := fn(num, typ, b[:n]); err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}
