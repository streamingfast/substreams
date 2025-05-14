import bigInt from "../../bench/substreams_ts/shims/bigInt"

import { BlockSchema, TransactionTraceStatus } from "./pb/sf/ethereum/type/v2/type_pb"
import {
    DatabaseChanges,
    DatabaseChangesSchema,
    FieldSchema,
    TableChange,
    TableChange_Operation,
    TableChangeSchema,
} from "./pb/sf/substreams/sink/database/v1/database_pb"
import { create, fromBinary, toBinary, toJson } from "@bufbuild/protobuf";

const rocketAddress = bytesFromHex("0xae78736Cd615f374D3085123A210448E74Fc6393")
const approvalTopic = bytesFromHex("0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925")
const transferTopic = bytesFromHex("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

export function main() {
    const out = map_block(readInput())
    writeOutput(out)
}

function readInput(): Uint8Array {
    return globalThis.input as Uint8Array
}

function writeOutput(output: any) {
    globalThis.output.set(output)
}

function map_noop() { }

function map_decode_proto_only(data: Uint8Array) {
    const block = fromBinary(BlockSchema, data)
}

function map_block(data: Uint8Array): any {
    const now = Date.now();
    const block = fromBinary(BlockSchema, data)
    const end = Date.now();
    console.log("decode time", end - now, "ms")
    const changes = create(DatabaseChangesSchema);

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

                    const change = create(TableChangeSchema);
                    change.table = "Approval"
                    change.primaryKey = { case: "pk", value: `${bytesToHex(trace.hash)}-${log.index}` }
                    change.operation = TableChange_Operation.CREATE
                    change.ordinal = bigInt(0) as unknown as bigint
                    change.fields = [
                        create(FieldSchema, { name: "timestamp", newValue: blockTimestampStr }),
                        create(FieldSchema, { name: "block_number", newValue: blockNumberStr }),
                        create(FieldSchema, { name: "log_index", newValue: log.index.toString() }),
                        create(FieldSchema, { name: "tx_hash", newValue: bytesToHex(trace.hash) }),
                        create(FieldSchema, { name: "spender", newValue: bytesToHex(log.topics[1].slice(12)) }),
                        create(FieldSchema, { name: "owner", newValue: bytesToHex(log.topics[2].slice(12)) }),
                        create(FieldSchema, { name: "amount", newValue: bytesToHex(stripZeroBytes(log.data)) }),
                    ]

                    changes.tableChanges.push(change)
                    return
                }

                if (bytesEqual(log.topics[0], transferTopic)) {
                    transferCount++

                    const change = create(TableChangeSchema)
                    change.table = "Transfer"
                    change.primaryKey = { case: "pk", value: `${bytesToHex(trace.hash)}-${log.index}` }
                    change.operation = TableChange_Operation.CREATE
                    change.ordinal = bigInt(0) as unknown as bigint
                    change.fields = [
                        create(FieldSchema, { name: "timestamp", newValue: blockTimestampStr }),
                        create(FieldSchema, { name: "block_number", newValue: blockNumberStr }),
                        create(FieldSchema, { name: "log_index", newValue: log.index.toString() }),
                        create(FieldSchema, { name: "tx_hash", newValue: bytesToHex(trace.hash) }),
                        create(FieldSchema, { name: "spender", newValue: bytesToHex(log.topics[1].slice(12)) }),
                        create(FieldSchema, { name: "owner", newValue: bytesToHex(log.topics[2].slice(12)) }),
                        create(FieldSchema, { name: "amount", newValue: bytesToHex(stripZeroBytes(log.data)) }),
                    ]

                    changes.tableChanges.push(change)
                    return
                }
            })
        })
    })

    return {
        trxCount,
        transferCount,
        approvalCount,
    }
}

function stripZeroBytes(input: Uint8Array): Uint8Array {
    for (let i = 0; i !== input.length; i++) {
        if (input[i] !== 0) {
            return input.slice(i)
        }
    }
    return input
}

function byteToHex(byte) {
    const unsignedByte = byte & 0xff
    return unsignedByte < 16 ? "0" + unsignedByte.toString(16) : unsignedByte.toString(16)
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
    return String.fromCharCode(...(chars as unknown as number[]))
}

function bytesFromHex(hex: string): Uint8Array {
    if (hex.match(/^0(x|X)/)) hex = hex.slice(2)
    if (hex.length % 2 !== 0) hex = "0" + hex

    const bytes = new Uint8Array(hex.length / 2)
    for (let i = 0, c = 0; c < hex.length; c += 2, i++) {
        bytes[i] = parseInt(hex.slice(c, c + 2), 16)
    }
    return bytes
}

function bytesEqual(left: Uint8Array, right: Uint8Array) {
    if (left.length !== right.length) return false
    for (let i = 0; i < left.byteLength; i++) {
        if (left[i] !== right[i]) return false
    }
    return true
}
