// Package rows turns a protobuf module output into an ordered stream of typed rows.
//
// The package is stateless per block: no reorg journal, no event ids, no finality and no delivery.
// A [RowProducer] resolves a [Schema] (tables, columns, logical types, keys) once and yields the rows of
// each block as a [Stream], a one-pass iterator of [Row] views that point into the decoded message.
// Values are raw [protoreflect.Value]s: nothing is converted, boxed or copied by the extractor, which only
// reads the message through the read-only part of [protoreflect.Message] (Descriptor, Get, Has, List),
// so dynamicpb and hyperpb messages both work.
//
// A row, and every string or byte slice reachable from it, is only valid until the next step of the
// iteration, see [Row.Detach] for what a copy does and does not keep alive.
package rows
