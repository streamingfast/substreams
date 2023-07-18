use pb::sf::{
    ethereum::r#type::v2::{Block, TransactionTraceStatus},
    substreams::sink::database::v1::{
        table_change::{Operation, PrimaryKey},
        DatabaseChanges, Field, TableChange,
    },
};
use substreams::{errors::Error, hex, Hex};

mod pb;

#[cfg(target_arch = "wasm32")]
#[no_mangle]
pub extern "C" fn map_noop(_params_ptr: *mut u8, _params_len: usize) {}

pub const CONTRACT: [u8; 20] = hex!("ae78736Cd615f374D3085123A210448E74Fc6393");

pub const APPROVAL_TOPIC: [u8; 32] =
    hex!("8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925");
pub const TRANSFER_TOPIC: [u8; 32] =
    hex!("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef");

#[cfg(target_arch = "wasm32")]
#[no_mangle]
pub extern "C" fn map_decode_proto_only(blk_ptr: *mut u8, blk_len: usize) {
    let _blk: Block = substreams::proto::decode_ptr(blk_ptr, blk_len).unwrap();
}

#[substreams::handlers::map]
fn map_block(blk: Block) -> Result<DatabaseChanges, Error> {
    let mut changes = DatabaseChanges {
        table_changes: vec![],
    };

    let block_number_str = blk.header.as_ref().unwrap().number.to_string();
    let block_timestamp_str = blk
        .header
        .as_ref()
        .unwrap()
        .timestamp
        .as_ref()
        .unwrap()
        .seconds
        .to_string();

    for trx in blk.transaction_traces {
        if trx.status != TransactionTraceStatus::Succeeded as i32 {
            continue;
        }

        for call in trx.calls {
            if call.state_reverted {
                continue;
            }

            for log in call.logs {
                if log.address != CONTRACT || log.topics.len() == 0 {
                    continue;
                }

                if log.topics.get(0).unwrap() == &APPROVAL_TOPIC {
                    changes.table_changes.push(TableChange {
                        table: "Approval".to_string(),
                        primary_key: Some(PrimaryKey::Pk(format!(
                            "{}-{}",
                            Hex(&trx.hash),
                            log.index
                        ))),
                        operation: Operation::Create as i32,
                        ordinal: 0,
                        fields: vec![
                            Field {
                                name: "timestamp".to_string(),
                                old_value: "".to_string(),
                                new_value: block_timestamp_str.clone(),
                            },
                            Field {
                                name: "block_number".to_string(),
                                old_value: "".to_string(),
                                new_value: block_number_str.clone(),
                            },
                            Field {
                                name: "log_index".to_string(),
                                old_value: "".to_string(),
                                new_value: log.index.to_string(),
                            },
                            Field {
                                name: "tx_hash".to_string(),
                                old_value: "".to_string(),
                                new_value: Hex(&trx.hash).to_string(),
                            },
                            Field {
                                name: "spender".to_string(),
                                old_value: "".to_string(),
                                new_value: Hex(&log.topics[1][12..]).to_string(),
                            },
                            Field {
                                name: "owner".to_string(),
                                old_value: "".to_string(),
                                new_value: Hex(&log.topics[2][12..]).to_string(),
                            },
                            Field {
                                name: "amount".to_string(),
                                old_value: "".to_string(),
                                new_value: Hex(strip_zero_bytes(log.data.as_slice())).to_string(),
                            },
                        ],
                    });

                    continue;
                }

                if log.topics.get(0).unwrap() == &TRANSFER_TOPIC {
                    changes.table_changes.push(TableChange {
                        table: "Tranfer".to_string(),
                        primary_key: Some(PrimaryKey::Pk(format!(
                            "{}-{}",
                            Hex(&trx.hash),
                            log.index
                        ))),
                        operation: Operation::Create as i32,
                        ordinal: 0,
                        fields: vec![
                            Field {
                                name: "timestamp".to_string(),
                                old_value: "".to_string(),
                                new_value: block_timestamp_str.clone(),
                            },
                            Field {
                                name: "block_number".to_string(),
                                old_value: "".to_string(),
                                new_value: block_number_str.clone(),
                            },
                            Field {
                                name: "log_index".to_string(),
                                old_value: "".to_string(),
                                new_value: log.index.to_string(),
                            },
                            Field {
                                name: "tx_hash".to_string(),
                                old_value: "".to_string(),
                                new_value: Hex(&trx.hash).to_string(),
                            },
                            Field {
                                name: "sender".to_string(),
                                old_value: "".to_string(),
                                new_value: Hex(&log.topics[1][12..]).to_string(),
                            },
                            Field {
                                name: "receiver".to_string(),
                                old_value: "".to_string(),
                                new_value: Hex(&log.topics[2][12..]).to_string(),
                            },
                            Field {
                                name: "value".to_string(),
                                old_value: "".to_string(),
                                new_value: Hex(strip_zero_bytes(log.data.as_slice())).to_string(),
                            },
                        ],
                    });

                    continue;
                }
            }
        }
    }

    Ok(changes)
}

fn strip_zero_bytes(input: &[u8]) -> &[u8] {
    for n in 0..input.len() {
        if input[n] != 0 {
            return &input[n..];
        }
    }

    input
}
