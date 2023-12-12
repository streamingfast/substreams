import bigInt from "./shims/bigInt"

import { Block, TransactionTraceStatus } from "./pb/sf/ethereum/type/v2/type_pb"
import {
  DatabaseChanges,
  Field,
  TableChange,
  TableChange_Operation,
} from "./pb/sf/substreams/sink/database/v1/database_pb"

const rocketAddress = bytesFromHex("0xae78736Cd615f374D3085123A210448E74Fc6393")
const approvalTopic = bytesFromHex(
  "0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925",
)
const transferTopic = bytesFromHex(
  "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
)

// @ts-ignore

export function popo() {
  console.log("Hello from popo!")


  const out = map_block(readInput())
  writeOutput(out)
}

// Read input from stdin
function readInput(): Uint8Array {
  const chunkSize = 1 * 1024 * 1024
  const inputChunks = []
  let totalBytes = 0

  // Read all the available bytes
  while (1) {
    const buffer = new Uint8Array(chunkSize)
    // Stdin file descriptor
    const fd = 0
    // @ts-ignore
    const bytesRead = Javy.IO.readSync(fd, buffer)

    totalBytes += bytesRead
    if (bytesRead === 0) {
      break
    }
    inputChunks.push(buffer.subarray(0, bytesRead))
  }

  // Assemble input into a single Uint8Array
  const { finalBuffer } = inputChunks.reduce(
    (context, chunk) => {
      context.finalBuffer.set(chunk, context.bufferOffset)
      context.bufferOffset += chunk.length
      return context
    },
    { bufferOffset: 0, finalBuffer: new Uint8Array(totalBytes) },
  )

  return finalBuffer
}

function writeOutput(output: any) {
  const encodedOutput = new TextEncoder().encode(JSON.stringify(output))
  const buffer = new Uint8Array(encodedOutput)
  // Stdout file descriptor
  const fd = 1

  // @ts-ignore
  Javy.IO.writeSync(fd, buffer)
}

function map_noop() {}

function map_decode_proto_only(data: Uint8Array) {
  const block = new Block()
  block.fromBinary(data)
}

function map_block(data: Uint8Array): any {
  const block = new Block()
  block.fromBinary(data)

  const changes = new DatabaseChanges()

  const blockNumberStr = block.header?.number.toString() ?? ""
  const blockTimestampStr = block.header?.timestamp?.seconds.toString() ?? ""

  let trxCount = 0
  let transferCount = 0
  let approvalCount = 0

  block.transactionTraces.forEach((trace) => {
    trxCount++

    if (trace.status !== TransactionTraceStatus.SUCCEEDED) {
      return
    }

    trace.calls.forEach((call) => {
      if (call.stateReverted) {
        return
      }

      call.logs.forEach((log) => {
        if (!bytesEqual(log.address, rocketAddress) || log.topics.length === 0) {
          return
        }

        if (bytesEqual(log.topics[0], approvalTopic)) {
          approvalCount++

          const change = new TableChange()
          change.table = "Approval"
          change.primaryKey = { case: "pk", value: `${bytesToHex(trace.hash)}-${log.index}` }
          change.operation = TableChange_Operation.CREATE
          change.ordinal = bigInt(0) as unknown as bigint
          change.fields = [
            new Field({ name: "timestamp", newValue: blockTimestampStr }),
            new Field({ name: "block_number", newValue: blockNumberStr }),
            new Field({ name: "log_index", newValue: log.index.toString() }),
            new Field({ name: "tx_hash", newValue: bytesToHex(trace.hash) }),
            new Field({ name: "spender", newValue: bytesToHex(log.topics[1].slice(12)) }),
            new Field({ name: "owner", newValue: bytesToHex(log.topics[2].slice(12)) }),
            new Field({ name: "amount", newValue: bytesToHex(stripZeroBytes(log.data)) }),
          ]

          changes.tableChanges.push(change)
          return
        }

        if (bytesEqual(log.topics[0], transferTopic)) {
          transferCount++

          const change = new TableChange({})
          change.table = "Transfer"
          change.primaryKey = { case: "pk", value: `${bytesToHex(trace.hash)}-${log.index}` }
          change.operation = TableChange_Operation.CREATE
          change.ordinal = bigInt(0) as unknown as bigint
          change.fields = [
            new Field({ name: "timestamp", newValue: blockTimestampStr }),
            new Field({ name: "block_number", newValue: blockNumberStr }),
            new Field({ name: "log_index", newValue: log.index.toString() }),
            new Field({ name: "tx_hash", newValue: bytesToHex(trace.hash) }),
            new Field({ name: "sender", newValue: bytesToHex(log.topics[1].slice(12)) }),
            new Field({ name: "receiver", newValue: bytesToHex(log.topics[2].slice(12)) }),
            new Field({ name: "value", newValue: bytesToHex(stripZeroBytes(log.data)) }),
          ]

          changes.tableChanges.push(change)
          return
        }
      })
    })
  })

  // substreams_engine.output(changes.toBinary())

  return {
    trxCount,
    transferCount,
    approvalCount,
  }
}

function stripZeroBytes(input: Uint8Array): Uint8Array {
  for (let i = 0; i != input.length; i++) {
    if (input[i] != 0) {
      return input.slice(i)
    }
  }

  return input
}

function byteToHex(byte) {
  // convert the possibly signed byte (-128 to 127) to an unsigned byte (0 to 255).
  // if you know, that you only deal with unsigned bytes (Uint8Array), you can omit this line
  const unsignedByte = byte & 0xff

  // If the number can be represented with only 4 bits (0-15),
  // the hexadecimal representation of this number is only one char (0-9, a-f).
  if (unsignedByte < 16) {
    return "0" + unsignedByte.toString(16)
  } else {
    return unsignedByte.toString(16)
  }
}

const alphaCharCode = "a".charCodeAt(0) - 10
const digitCharCode = "0".charCodeAt(0)

function bytesToHex(byteArray: Uint8Array) {
  const chars = new Uint8Array(byteArray.length * 2)

  let p = 0
  for (let i = 0; i < byteArray.length; i++) {
    let nibble = byteArray[i] >>> 4
    chars[p++] = nibble > 9 ? nibble + alphaCharCode : nibble + digitCharCode
    nibble = byteArray[i] & 0xf
    chars[p++] = nibble > 9 ? nibble + alphaCharCode : nibble + digitCharCode
  }

  return String.fromCharCode.apply(null, chars as unknown as number[])
}
// function bytesToHex(input: Uint8Array): string {
//   return Buffer.from(input).toString("hex")
// }

// FIXME: If we keep this, let's refactor to use the inverse logic of `bytesToHex` for decoding,
// it's faster then going with `parseInt`
function bytesFromHex(hex: string): Uint8Array {
  if (hex.match(/^0(x|X)/)) {
    hex = hex.slice(2)
  }

  if (hex.length % 2 !== 0) {
    hex = "0" + hex
  }

  let i = 0
  let bytes = new Uint8Array(hex.length / 2)
  for (let c = 0; c < hex.length; c += 2) {
    bytes[i] = parseInt(hex.slice(c, c + 2), 16)
    i++
  }

  return bytes
}

// function bytesFromHex(input: string): Uint8Array {
//   if (input.match(/^0(x|X)/)) {
//     input = input.slice(2)
//   }

//   return new Uint8Array(Buffer.from(input, "hex"))
// }

function bytesEqual(left: Uint8Array, right: Uint8Array) {
  if (left.length != right.length) return false

  for (var i = 0; i != left.byteLength; i++) {
    if (left[i] != right[i]) return false
  }

  return true
}
