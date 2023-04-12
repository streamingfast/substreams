mod abi;
mod pb;
use hex_literal::hex;
use pb::contract::v1 as contract;
use substreams::Hex;
use substreams_ethereum::pb::eth::v2 as eth;
use substreams_ethereum::Event;

#[allow(unused_imports)]
use num_traits::cast::ToPrimitive;

const TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

substreams_ethereum::init!();

#[substreams::handlers::map]
fn map_events(blk: eth::Block) -> Result<contract::Events, substreams::errors::Error> {
    Ok(contract::Events {
        approvals: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) = abi::contract::events::Approval::match_and_decode(log) {
                        return Some(contract::Approval {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
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
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) = abi::contract::events::ApprovalForAll::match_and_decode(log) {
                        return Some(contract::ApprovalForAll {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
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
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) = abi::contract::events::OwnershipTransferred::match_and_decode(log) {
                        return Some(contract::OwnershipTransferred {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
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
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) = abi::contract::events::Transfer::match_and_decode(log) {
                        return Some(contract::Transfer {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
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
