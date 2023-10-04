mod abi;
mod pb;
use hex_literal::hex;
use pb::contract::v1 as contract;
use substreams::Hex;
use substreams_database_change::pb::database::DatabaseChanges;
use substreams_ethereum::pb::eth::v2 as eth;
use substreams_ethereum::Event;

#[allow(unused_imports)]
use num_traits::cast::ToPrimitive;

const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

substreams_ethereum::init!();

#[substreams::handlers::map]
fn map_events(blk: eth::Block) -> Result<contract::Events, substreams::errors::Error> {
    let timestamp_s = blk.timestamp().seconds as u64 + blk.timestamp().nanos as u64;

    Ok(contract::Events {
        approvals: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter()
                    .filter(|log| log.address == TRACKED_CONTRACT)
                    .filter_map(|log| {
                        if let Some(event) = abi::contract::events::Approval::match_and_decode(log) {
                            return Some(contract::Approval {
                                trx_hash: Hex(&view.transaction.hash).to_string(),
                                log_index: log.block_index,
                                timestamp_s,
                                block_num: blk.number,
                                approved: event.approved,
                                owner: event.owner,
                                token_id: event.token_id.to_string(),
                            });
                        }

                        None
                })
            })
            .collect(),
        approval_for_alls: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter()
                    .filter(|log| log.address == TRACKED_CONTRACT)
                    .filter_map(|log| {
                        if let Some(event) = abi::contract::events::ApprovalForAll::match_and_decode(log) {
                            return Some(contract::ApprovalForAll {
                                trx_hash: Hex(&view.transaction.hash).to_string(),
                                log_index: log.block_index,
                                timestamp_s,
                                block_num: blk.number,
                                approved: event.approved,
                                operator: event.operator,
                                owner: event.owner,
                            });
                        }

                        None
                })
            })
            .collect(),
        ownership_transferreds: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter()
                    .filter(|log| log.address == TRACKED_CONTRACT)
                    .filter_map(|log| {
                        if let Some(event) = abi::contract::events::OwnershipTransferred::match_and_decode(log) {
                            return Some(contract::OwnershipTransferred {
                                trx_hash: Hex(&view.transaction.hash).to_string(),
                                log_index: log.block_index,
                                timestamp_s,
                                block_num: blk.number,
                                new_owner: event.new_owner,
                                previous_owner: event.previous_owner,
                            });
                        }

                        None
                })
            })
            .collect(),
        transfers: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter()
                    .filter(|log| log.address == TRACKED_CONTRACT)
                    .filter_map(|log| {
                        if let Some(event) = abi::contract::events::Transfer::match_and_decode(log) {
                            return Some(contract::Transfer {
                                trx_hash: Hex(&view.transaction.hash).to_string(),
                                log_index: log.block_index,
                                timestamp_s,
                                block_num: blk.number,
                                from: event.from,
                                to: event.to,
                                token_id: event.token_id.to_string(),
                            });
                        }

                        None
                })
            })
            .collect(),
    })
}

#[substreams::handlers::map]
fn db_out(events: contract::Events) -> Result<DatabaseChanges, substreams::errors::Error> {
    // Initialize Database Changes container
    let mut tables = substreams_database_change::tables::Tables::new();

    // Loop over all the abis events to create database changes
    events.approvals.into_iter().for_each(|evt| {
        tables
            .create_row("approvals", format!("{}-{}", evt.trx_hash, evt.log_index))
            .set("trx_hash", evt.trx_hash)
            .set("log_index", evt.log_index)
            .set("timestamp_s", evt.timestamp_s)
            .set("block_num", evt.block_num)
            .set("approved", Hex(&evt.approved).to_string())
            .set("owner", Hex(&evt.owner).to_string())
            .set("token_id", evt.token_id.to_string());
    });
    events.approval_for_alls.into_iter().for_each(|evt| {
        tables
            .create_row("approval_for_alls", format!("{}-{}", evt.trx_hash, evt.log_index))
            .set("trx_hash", evt.trx_hash)
            .set("log_index", evt.log_index)
            .set("timestamp_s", evt.timestamp_s)
            .set("block_num", evt.block_num)
            .set("approved", evt.approved)
            .set("operator", Hex(&evt.operator).to_string())
            .set("owner", Hex(&evt.owner).to_string());
    });
    events.ownership_transferreds.into_iter().for_each(|evt| {
        tables
            .create_row("ownership_transferreds", format!("{}-{}", evt.trx_hash, evt.log_index))
            .set("trx_hash", evt.trx_hash)
            .set("log_index", evt.log_index)
            .set("timestamp_s", evt.timestamp_s)
            .set("block_num", evt.block_num)
            .set("new_owner", Hex(&evt.new_owner).to_string())
            .set("previous_owner", Hex(&evt.previous_owner).to_string());
    });
    events.transfers.into_iter().for_each(|evt| {
        tables
            .create_row("transfers", format!("{}-{}", evt.trx_hash, evt.log_index))
            .set("trx_hash", evt.trx_hash)
            .set("log_index", evt.log_index)
            .set("timestamp_s", evt.timestamp_s)
            .set("block_num", evt.block_num)
            .set("from", Hex(&evt.from).to_string())
            .set("to", Hex(&evt.to).to_string())
            .set("token_id", evt.token_id.to_string());
    });

    Ok(tables.to_database_changes())
}
