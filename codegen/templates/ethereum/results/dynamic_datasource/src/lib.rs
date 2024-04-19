mod abi;
mod pb;
use hex_literal::hex;
use pb::contract::v1 as contract;
use substreams::prelude::*;
use substreams::store;
use substreams::Hex;
use substreams_database_change::pb::database::DatabaseChanges;
use substreams_database_change::tables::Tables as DatabaseChangeTables;
use substreams_entity_change::pb::entity::EntityChanges;
use substreams_entity_change::tables::Tables as EntityChangesTables;
use substreams_ethereum::pb::eth::v2 as eth;
use substreams_ethereum::Event;

#[allow(unused_imports)]
use num_traits::cast::ToPrimitive;
use std::str::FromStr;
use substreams::scalar::BigDecimal;

substreams_ethereum::init!();

const FACTORY_TRACKED_CONTRACT: [u8; 20] = hex!("1f98431c8ad98523631ae4a59f267346ea31f984");

fn map_factory_events(blk: &eth::Block, events: &mut contract::Events) {
    events.factory_fee_amount_enableds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == FACTORY_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::factory_contract::events::FeeAmountEnabled::match_and_decode(log) {
                        return Some(contract::FactoryFeeAmountEnabled {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            fee: event.fee.to_u64(),
                            tick_spacing: Into::<num_bigint::BigInt>::into(event.tick_spacing).to_i64().unwrap(),
                        });
                    }

                    None
                })
        })
        .collect());
    events.factory_owner_changeds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == FACTORY_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::factory_contract::events::OwnerChanged::match_and_decode(log) {
                        return Some(contract::FactoryOwnerChanged {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            new_owner: event.new_owner,
                            old_owner: event.old_owner,
                        });
                    }

                    None
                })
        })
        .collect());
    events.factory_pool_createds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == FACTORY_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::factory_contract::events::PoolCreated::match_and_decode(log) {
                        return Some(contract::FactoryPoolCreated {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            fee: event.fee.to_u64(),
                            pool: event.pool,
                            tick_spacing: Into::<num_bigint::BigInt>::into(event.tick_spacing).to_i64().unwrap(),
                            token0: event.token0,
                            token1: event.token1,
                        });
                    }

                    None
                })
        })
        .collect());
}

fn is_declared_dds_address(addr: &Vec<u8>, ordinal: u64, dds_store: &store::StoreGetInt64) -> bool {
    //    substreams::log::info!("Checking if address {} is declared dds address", Hex(addr).to_string());
    if dds_store.get_at(ordinal, Hex(addr).to_string()).is_some() {
        return true;
    }
    return false;
}

fn map_pool_events(
    blk: &eth::Block,
    dds_store: &store::StoreGetInt64,
    events: &mut contract::Events,
) {

    events.pool_burns.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::Burn::match_and_decode(log) {
                        return Some(contract::PoolBurn {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            amount: event.amount.to_string(),
                            amount0: event.amount0.to_string(),
                            amount1: event.amount1.to_string(),
                            owner: event.owner,
                            tick_lower: Into::<num_bigint::BigInt>::into(event.tick_lower).to_i64().unwrap(),
                            tick_upper: Into::<num_bigint::BigInt>::into(event.tick_upper).to_i64().unwrap(),
                        });
                    }

                    None
                })
        })
        .collect());

    events.pool_collects.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::Collect::match_and_decode(log) {
                        return Some(contract::PoolCollect {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            amount0: event.amount0.to_string(),
                            amount1: event.amount1.to_string(),
                            owner: event.owner,
                            recipient: event.recipient,
                            tick_lower: Into::<num_bigint::BigInt>::into(event.tick_lower).to_i64().unwrap(),
                            tick_upper: Into::<num_bigint::BigInt>::into(event.tick_upper).to_i64().unwrap(),
                        });
                    }

                    None
                })
        })
        .collect());

    events.pool_collect_protocols.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::CollectProtocol::match_and_decode(log) {
                        return Some(contract::PoolCollectProtocol {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            amount0: event.amount0.to_string(),
                            amount1: event.amount1.to_string(),
                            recipient: event.recipient,
                            sender: event.sender,
                        });
                    }

                    None
                })
        })
        .collect());

    events.pool_flashes.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::Flash::match_and_decode(log) {
                        return Some(contract::PoolFlash {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            amount0: event.amount0.to_string(),
                            amount1: event.amount1.to_string(),
                            paid0: event.paid0.to_string(),
                            paid1: event.paid1.to_string(),
                            recipient: event.recipient,
                            sender: event.sender,
                        });
                    }

                    None
                })
        })
        .collect());

    events.pool_increase_observation_cardinality_nexts.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::IncreaseObservationCardinalityNext::match_and_decode(log) {
                        return Some(contract::PoolIncreaseObservationCardinalityNext {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            observation_cardinality_next_new: event.observation_cardinality_next_new.to_u64(),
                            observation_cardinality_next_old: event.observation_cardinality_next_old.to_u64(),
                        });
                    }

                    None
                })
        })
        .collect());

    events.pool_initializes.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::Initialize::match_and_decode(log) {
                        return Some(contract::PoolInitialize {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            sqrt_price_x96: event.sqrt_price_x96.to_string(),
                            tick: Into::<num_bigint::BigInt>::into(event.tick).to_i64().unwrap(),
                        });
                    }

                    None
                })
        })
        .collect());

    events.pool_mints.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::Mint::match_and_decode(log) {
                        return Some(contract::PoolMint {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            amount: event.amount.to_string(),
                            amount0: event.amount0.to_string(),
                            amount1: event.amount1.to_string(),
                            owner: event.owner,
                            sender: event.sender,
                            tick_lower: Into::<num_bigint::BigInt>::into(event.tick_lower).to_i64().unwrap(),
                            tick_upper: Into::<num_bigint::BigInt>::into(event.tick_upper).to_i64().unwrap(),
                        });
                    }

                    None
                })
        })
        .collect());

    events.pool_set_fee_protocols.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::SetFeeProtocol::match_and_decode(log) {
                        return Some(contract::PoolSetFeeProtocol {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            fee_protocol0_new: event.fee_protocol0_new.to_u64(),
                            fee_protocol0_old: event.fee_protocol0_old.to_u64(),
                            fee_protocol1_new: event.fee_protocol1_new.to_u64(),
                            fee_protocol1_old: event.fee_protocol1_old.to_u64(),
                        });
                    }

                    None
                })
        })
        .collect());

    events.pool_swaps.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| is_declared_dds_address(&log.address, log.ordinal, dds_store))
                .filter_map(|log| {
                    if let Some(event) = abi::pool_contract::events::Swap::match_and_decode(log) {
                        return Some(contract::PoolSwap {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            evt_address: Hex(&log.address).to_string(),
                            amount0: event.amount0.to_string(),
                            amount1: event.amount1.to_string(),
                            liquidity: event.liquidity.to_string(),
                            recipient: event.recipient,
                            sender: event.sender,
                            sqrt_price_x96: event.sqrt_price_x96.to_string(),
                            tick: Into::<num_bigint::BigInt>::into(event.tick).to_i64().unwrap(),
                        });
                    }

                    None
                })
        })
        .collect());
}


fn db_factory_out(events: &contract::Events, tables: &mut DatabaseChangeTables) {
    // Loop over all the abis events to create table changes
    events.factory_fee_amount_enableds.iter().for_each(|evt| {
        tables
            .create_row("factory_fee_amount_enabled", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("fee", evt.fee)
            .set("tick_spacing", evt.tick_spacing);
    });
    events.factory_owner_changeds.iter().for_each(|evt| {
        tables
            .create_row("factory_owner_changed", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("new_owner", Hex(&evt.new_owner).to_string())
            .set("old_owner", Hex(&evt.old_owner).to_string());
    });
    events.factory_pool_createds.iter().for_each(|evt| {
        tables
            .create_row("factory_pool_created", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("fee", evt.fee)
            .set("pool", Hex(&evt.pool).to_string())
            .set("tick_spacing", evt.tick_spacing)
            .set("token0", Hex(&evt.token0).to_string())
            .set("token1", Hex(&evt.token1).to_string());
    });
}
fn db_pool_out(events: &contract::Events, tables: &mut DatabaseChangeTables) {
    // Loop over all the abis events to create table changes
    events.pool_burns.iter().for_each(|evt| {
        tables
            .create_row("pool_burn", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount", BigDecimal::from_str(&evt.amount).unwrap())
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("owner", Hex(&evt.owner).to_string())
            .set("tick_lower", evt.tick_lower)
            .set("tick_upper", evt.tick_upper);
    });
    events.pool_collects.iter().for_each(|evt| {
        tables
            .create_row("pool_collect", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("owner", Hex(&evt.owner).to_string())
            .set("recipient", Hex(&evt.recipient).to_string())
            .set("tick_lower", evt.tick_lower)
            .set("tick_upper", evt.tick_upper);
    });
    events.pool_collect_protocols.iter().for_each(|evt| {
        tables
            .create_row("pool_collect_protocol", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("recipient", Hex(&evt.recipient).to_string())
            .set("sender", Hex(&evt.sender).to_string());
    });
    events.pool_flashes.iter().for_each(|evt| {
        tables
            .create_row("pool_flash", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("paid0", BigDecimal::from_str(&evt.paid0).unwrap())
            .set("paid1", BigDecimal::from_str(&evt.paid1).unwrap())
            .set("recipient", Hex(&evt.recipient).to_string())
            .set("sender", Hex(&evt.sender).to_string());
    });
    events.pool_increase_observation_cardinality_nexts.iter().for_each(|evt| {
        tables
            .create_row("pool_increase_observation_cardinality_next", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("observation_cardinality_next_new", evt.observation_cardinality_next_new)
            .set("observation_cardinality_next_old", evt.observation_cardinality_next_old);
    });
    events.pool_initializes.iter().for_each(|evt| {
        tables
            .create_row("pool_initialize", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("sqrt_price_x96", BigDecimal::from_str(&evt.sqrt_price_x96).unwrap())
            .set("tick", evt.tick);
    });
    events.pool_mints.iter().for_each(|evt| {
        tables
            .create_row("pool_mint", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount", BigDecimal::from_str(&evt.amount).unwrap())
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("owner", Hex(&evt.owner).to_string())
            .set("sender", Hex(&evt.sender).to_string())
            .set("tick_lower", evt.tick_lower)
            .set("tick_upper", evt.tick_upper);
    });
    events.pool_set_fee_protocols.iter().for_each(|evt| {
        tables
            .create_row("pool_set_fee_protocol", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("fee_protocol0_new", evt.fee_protocol0_new)
            .set("fee_protocol0_old", evt.fee_protocol0_old)
            .set("fee_protocol1_new", evt.fee_protocol1_new)
            .set("fee_protocol1_old", evt.fee_protocol1_old);
    });
    events.pool_swaps.iter().for_each(|evt| {
        tables
            .create_row("pool_swap", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("liquidity", BigDecimal::from_str(&evt.liquidity).unwrap())
            .set("recipient", Hex(&evt.recipient).to_string())
            .set("sender", Hex(&evt.sender).to_string())
            .set("sqrt_price_x96", BigDecimal::from_str(&evt.sqrt_price_x96).unwrap())
            .set("tick", evt.tick);
    });
}


fn graph_factory_out(events: &contract::Events, tables: &mut EntityChangesTables) {
    // Loop over all the abis events to create table changes
    events.factory_fee_amount_enableds.iter().for_each(|evt| {
        tables
            .create_row("factory_fee_amount_enabled", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("fee", evt.fee)
            .set("tick_spacing", evt.tick_spacing);
    });
    events.factory_owner_changeds.iter().for_each(|evt| {
        tables
            .create_row("factory_owner_changed", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("new_owner", Hex(&evt.new_owner).to_string())
            .set("old_owner", Hex(&evt.old_owner).to_string());
    });
    events.factory_pool_createds.iter().for_each(|evt| {
        tables
            .create_row("factory_pool_created", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("fee", evt.fee)
            .set("pool", Hex(&evt.pool).to_string())
            .set("tick_spacing", evt.tick_spacing)
            .set("token0", Hex(&evt.token0).to_string())
            .set("token1", Hex(&evt.token1).to_string());
    });
}
fn graph_pool_out(events: &contract::Events, tables: &mut EntityChangesTables) {
    // Loop over all the abis events to create table changes
    events.pool_burns.iter().for_each(|evt| {
        tables
            .create_row("pool_burn", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount", BigDecimal::from_str(&evt.amount).unwrap())
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("owner", Hex(&evt.owner).to_string())
            .set("tick_lower", evt.tick_lower)
            .set("tick_upper", evt.tick_upper);
    });
    events.pool_collects.iter().for_each(|evt| {
        tables
            .create_row("pool_collect", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("owner", Hex(&evt.owner).to_string())
            .set("recipient", Hex(&evt.recipient).to_string())
            .set("tick_lower", evt.tick_lower)
            .set("tick_upper", evt.tick_upper);
    });
    events.pool_collect_protocols.iter().for_each(|evt| {
        tables
            .create_row("pool_collect_protocol", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("recipient", Hex(&evt.recipient).to_string())
            .set("sender", Hex(&evt.sender).to_string());
    });
    events.pool_flashes.iter().for_each(|evt| {
        tables
            .create_row("pool_flash", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("paid0", BigDecimal::from_str(&evt.paid0).unwrap())
            .set("paid1", BigDecimal::from_str(&evt.paid1).unwrap())
            .set("recipient", Hex(&evt.recipient).to_string())
            .set("sender", Hex(&evt.sender).to_string());
    });
    events.pool_increase_observation_cardinality_nexts.iter().for_each(|evt| {
        tables
            .create_row("pool_increase_observation_cardinality_next", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("observation_cardinality_next_new", evt.observation_cardinality_next_new)
            .set("observation_cardinality_next_old", evt.observation_cardinality_next_old);
    });
    events.pool_initializes.iter().for_each(|evt| {
        tables
            .create_row("pool_initialize", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("sqrt_price_x96", BigDecimal::from_str(&evt.sqrt_price_x96).unwrap())
            .set("tick", evt.tick);
    });
    events.pool_mints.iter().for_each(|evt| {
        tables
            .create_row("pool_mint", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount", BigDecimal::from_str(&evt.amount).unwrap())
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("owner", Hex(&evt.owner).to_string())
            .set("sender", Hex(&evt.sender).to_string())
            .set("tick_lower", evt.tick_lower)
            .set("tick_upper", evt.tick_upper);
    });
    events.pool_set_fee_protocols.iter().for_each(|evt| {
        tables
            .create_row("pool_set_fee_protocol", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("fee_protocol0_new", evt.fee_protocol0_new)
            .set("fee_protocol0_old", evt.fee_protocol0_old)
            .set("fee_protocol1_new", evt.fee_protocol1_new)
            .set("fee_protocol1_old", evt.fee_protocol1_old);
    });
    events.pool_swaps.iter().for_each(|evt| {
        tables
            .create_row("pool_swap", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("evt_address", &evt.evt_address)
            .set("amount0", BigDecimal::from_str(&evt.amount0).unwrap())
            .set("amount1", BigDecimal::from_str(&evt.amount1).unwrap())
            .set("liquidity", BigDecimal::from_str(&evt.liquidity).unwrap())
            .set("recipient", Hex(&evt.recipient).to_string())
            .set("sender", Hex(&evt.sender).to_string())
            .set("sqrt_price_x96", BigDecimal::from_str(&evt.sqrt_price_x96).unwrap())
            .set("tick", evt.tick);
    });
}
#[substreams::handlers::store]
fn store_factory_pool_created(blk: eth::Block, store: StoreSetInt64) {
    for rcpt in blk.receipts() {
        for log in rcpt
            .receipt
            .logs
            .iter()
            .filter(|log| log.address == FACTORY_TRACKED_CONTRACT)
        {
            if let Some(event) = abi::factory_contract::events::PoolCreated::match_and_decode(log) {
                store.set(log.ordinal, Hex(event.pool).to_string(), &1);
            }
        }
    }
}

#[substreams::handlers::map]
fn map_events(
    blk: eth::Block,
    store_pool: StoreGetInt64,
) -> Result<contract::Events, substreams::errors::Error> {
    let mut events = contract::Events::default();
    map_factory_events(&blk, &mut events);
    map_pool_events(&blk, &store_pool, &mut events);
    Ok(events)
}

#[substreams::handlers::map]
fn db_out(events: contract::Events) -> Result<DatabaseChanges, substreams::errors::Error> {
    // Initialize Database Changes container
    let mut tables = DatabaseChangeTables::new();
    db_factory_out(&events, &mut tables);
    db_pool_out(&events, &mut tables);
    Ok(tables.to_database_changes())
}

#[substreams::handlers::map]
fn graph_out(events: contract::Events) -> Result<EntityChanges, substreams::errors::Error> {
    // Initialize Database Changes container
    let mut tables = EntityChangesTables::new();
    graph_factory_out(&events, &mut tables);
    graph_pool_out(&events, &mut tables);
    Ok(tables.to_entity_changes())
}
