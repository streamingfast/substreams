// Package emitters registers every CDC output emitter (`file://`, `webhook(s)://`, `pubsub://`) by importing
// them, import it for its side effect:
//
//	import _ "github.com/streamingfast/substreams/sink/cdc/emitters"
package emitters

import (
	_ "github.com/streamingfast/substreams/sink/cdc/emitters/file"
	_ "github.com/streamingfast/substreams/sink/cdc/emitters/pubsub"
	_ "github.com/streamingfast/substreams/sink/cdc/emitters/webhook"
)
