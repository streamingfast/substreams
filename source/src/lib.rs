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

const FIOUU_TRACKED_CONTRACT: [u8; 20] = hex!("5c69bee701ef814a2b6a3edd4b1652cb9cc5aa6f");

fn map_fiouu_events(blk: &eth::Block, events: &mut contract::Events) {
    events.fiouu_pair_createds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == FIOUU_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::fiouu_contract::events::PairCreated::match_and_decode(log) {
                        return Some(contract::FiouuPairCreated {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            pair: event.pair,
                            param3: event.param3.to_string(),
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

fn db_fiouu_out(events: &contract::Events, tables: &mut DatabaseChangeTables) {
    // Loop over all the abis events to create table changes
    events.fiouu_pair_createds.iter().for_each(|evt| {
        tables
            .create_row("fiouu_pair_created", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("pair", Hex(&evt.pair).to_string())
            .set("param3", BigDecimal::from_str(&evt.param3).unwrap())
            .set("token0", Hex(&evt.token0).to_string())
            .set("token1", Hex(&evt.token1).to_string());
    });
}


fn graph_fiouu_out(events: &contract::Events, tables: &mut EntityChangesTables) {
    // Loop over all the abis events to create table changes
    events.fiouu_pair_createds.iter().for_each(|evt| {
        tables
            .create_row("fiouu_pair_created", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("pair", Hex(&evt.pair).to_string())
            .set("param3", BigDecimal::from_str(&evt.param3).unwrap())
            .set("token0", Hex(&evt.token0).to_string())
            .set("token1", Hex(&evt.token1).to_string());
    });
}

#[substreams::handlers::map]
fn map_events(
    blk: eth::Block,
) -> Result<contract::Events, substreams::errors::Error> {
    let mut events = contract::Events::default();
    map_fiouu_events(&blk, &mut events);
    Ok(events)
}

#[substreams::handlers::map]
fn db_out(events: contract::Events) -> Result<DatabaseChanges, substreams::errors::Error> {
    // Initialize Database Changes container
    let mut tables = DatabaseChangeTables::new();
    db_fiouu_out(&events, &mut tables);
    Ok(tables.to_database_changes())
}

#[substreams::handlers::map]
fn graph_out(events: contract::Events) -> Result<EntityChanges, substreams::errors::Error> {
    // Initialize Database Changes container
    let mut tables = EntityChangesTables::new();
    graph_fiouu_out(&events, &mut tables);
    Ok(tables.to_entity_changes())
}
