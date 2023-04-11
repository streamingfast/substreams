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
fn map_all_events(blk: eth::Block) -> Result<contract::Events, substreams::errors::Error> {
    Ok(contract::Events {
        addresses: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) = abi::contract::events::Address::match_and_decode(log) {
                        return Some(contract::Address {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
                            first: event.first,
                        });
                    }

                    None
                })
            })
            .collect(),
        array_of_addresses: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) =
                        abi::contract::events::ArrayOfAddress::match_and_decode(log)
                    {
                        return Some(contract::ArrayOfAddress {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
                            first: event.first.into_iter().map(|x| x).collect::<Vec<_>>(),
                        });
                    }

                    None
                })
            })
            .collect(),
        bytes: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) = abi::contract::events::Bytes::match_and_decode(log) {
                        return Some(contract::Bytes {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
                            first: event.first,
                        });
                    }

                    None
                })
            })
            .collect(),
        fixed_bytes: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) = abi::contract::events::FixedBytes::match_and_decode(log) {
                        return Some(contract::FixedBytes {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
                            first: Vec::from(event.first),
                        });
                    }

                    None
                })
            })
            .collect(),
        integers: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) = abi::contract::events::Integer::match_and_decode(log) {
                        return Some(contract::Integer {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
                            first: Into::<num_bigint::BigInt>::into(event.first)
                                .to_i64()
                                .unwrap(),
                        });
                    }

                    None
                })
            })
            .collect(),
        signed_fixed_points: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) =
                        abi::contract::events::SignedFixedPoint::match_and_decode(log)
                    {
                        return Some(contract::SignedFixedPoint {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
                            first: event.first.to_string(),
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
        unsigned_integers: blk
            .receipts()
            .flat_map(|view| {
                view.receipt.logs.iter().filter_map(|log| {
                    if log.address != TRACKED_CONTRACT {
                        return None;
                    }

                    if let Some(event) =
                        abi::contract::events::UnsignedInteger::match_and_decode(log)
                    {
                        return Some(contract::UnsignedInteger {
                            trx_hash: Hex(&view.transaction.hash).to_string(),
                            log_index: log.block_index,
                            first: event.first.to_string(),
                        });
                    }

                    None
                })
            })
            .collect(),
    })
}
