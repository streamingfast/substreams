mod abi;
mod pb;
use hex_literal::hex;
use pb::contract::v1 as contract;
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

const MOONBIRD_TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");
const BAYC_TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

fn map_moonbird_events(blk: &eth::Block, events: &mut contract::Events) {
    events.moonbird_approvals.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Approval::match_and_decode(log) {
                        return Some(contract::MoonbirdApproval {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            approved: event.approved,
                            owner: event.owner,
                            token_id: event.token_id.to_string(),
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_approval_for_alls.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::ApprovalForAll::match_and_decode(log) {
                        return Some(contract::MoonbirdApprovalForAll {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            approved: event.approved,
                            operator: event.operator,
                            owner: event.owner,
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_expelleds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Expelled::match_and_decode(log) {
                        return Some(contract::MoonbirdExpelled {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            token_id: event.token_id.to_string(),
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_nesteds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Nested::match_and_decode(log) {
                        return Some(contract::MoonbirdNested {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            token_id: event.token_id.to_string(),
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_ownership_transferreds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::OwnershipTransferred::match_and_decode(log) {
                        return Some(contract::MoonbirdOwnershipTransferred {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            new_owner: event.new_owner,
                            previous_owner: event.previous_owner,
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_pauseds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Paused::match_and_decode(log) {
                        return Some(contract::MoonbirdPaused {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            account: event.account,
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_refunds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Refund::match_and_decode(log) {
                        return Some(contract::MoonbirdRefund {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            amount: event.amount.to_string(),
                            buyer: event.buyer,
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_revenues.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Revenue::match_and_decode(log) {
                        return Some(contract::MoonbirdRevenue {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            amount: event.amount.to_string(),
                            beneficiary: event.beneficiary,
                            num_purchased: event.num_purchased.to_string(),
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_role_admin_changeds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::RoleAdminChanged::match_and_decode(log) {
                        return Some(contract::MoonbirdRoleAdminChanged {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            new_admin_role: Vec::from(event.new_admin_role),
                            previous_admin_role: Vec::from(event.previous_admin_role),
                            role: Vec::from(event.role),
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_role_granteds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::RoleGranted::match_and_decode(log) {
                        return Some(contract::MoonbirdRoleGranted {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            account: event.account,
                            role: Vec::from(event.role),
                            sender: event.sender,
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_role_revokeds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::RoleRevoked::match_and_decode(log) {
                        return Some(contract::MoonbirdRoleRevoked {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            account: event.account,
                            role: Vec::from(event.role),
                            sender: event.sender,
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_transfers.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Transfer::match_and_decode(log) {
                        return Some(contract::MoonbirdTransfer {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            from: event.from,
                            to: event.to,
                            token_id: event.token_id.to_string(),
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_unnesteds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Unnested::match_and_decode(log) {
                        return Some(contract::MoonbirdUnnested {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            token_id: event.token_id.to_string(),
                        });
                    }

                    None
                })
        })
        .collect());
    events.moonbird_unpauseds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == MOONBIRD_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::moonbird_contract::events::Unpaused::match_and_decode(log) {
                        return Some(contract::MoonbirdUnpaused {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            account: event.account,
                        });
                    }

                    None
                })
        })
        .collect());
}

fn map_bayc_events(blk: &eth::Block, events: &mut contract::Events) {
    events.bayc_approvals.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == BAYC_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::bayc_contract::events::Approval::match_and_decode(log) {
                        return Some(contract::BaycApproval {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            approved: event.approved,
                            owner: event.owner,
                            token_id: event.token_id.to_string(),
                        });
                    }

                    None
                })
        })
        .collect());
    events.bayc_approval_for_alls.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == BAYC_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::bayc_contract::events::ApprovalForAll::match_and_decode(log) {
                        return Some(contract::BaycApprovalForAll {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            approved: event.approved,
                            operator: event.operator,
                            owner: event.owner,
                        });
                    }

                    None
                })
        })
        .collect());
    events.bayc_ownership_transferreds.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == BAYC_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::bayc_contract::events::OwnershipTransferred::match_and_decode(log) {
                        return Some(contract::BaycOwnershipTransferred {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            new_owner: event.new_owner,
                            previous_owner: event.previous_owner,
                        });
                    }

                    None
                })
        })
        .collect());
    events.bayc_transfers.append(&mut blk
        .receipts()
        .flat_map(|view| {
            view.receipt.logs.iter()
                .filter(|log| log.address == BAYC_TRACKED_CONTRACT)
                .filter_map(|log| {
                    if let Some(event) = abi::bayc_contract::events::Transfer::match_and_decode(log) {
                        return Some(contract::BaycTransfer {
                            evt_tx_hash: Hex(&view.transaction.hash).to_string(),
                            evt_index: log.block_index,
                            evt_block_time: Some(blk.timestamp().to_owned()),
                            evt_block_number: blk.number,
                            from: event.from,
                            to: event.to,
                            token_id: event.token_id.to_string(),
                        });
                    }

                    None
                })
        })
        .collect());
}

fn db_moonbird_out(events: &contract::Events, tables: &mut DatabaseChangeTables) {
    // Loop over all the abis events to create table changes
    events.moonbird_approvals.iter().for_each(|evt| {
        tables
            .create_row("moonbird_approval", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("approved", Hex(&evt.approved).to_string())
            .set("owner", Hex(&evt.owner).to_string())
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_approval_for_alls.iter().for_each(|evt| {
        tables
            .create_row("moonbird_approval_for_all", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("approved", evt.approved)
            .set("operator", Hex(&evt.operator).to_string())
            .set("owner", Hex(&evt.owner).to_string());
    });
    events.moonbird_expelleds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_expelled", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_nesteds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_nested", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_ownership_transferreds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_ownership_transferred", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("new_owner", Hex(&evt.new_owner).to_string())
            .set("previous_owner", Hex(&evt.previous_owner).to_string());
    });
    events.moonbird_pauseds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_paused", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("account", Hex(&evt.account).to_string());
    });
    events.moonbird_refunds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_refund", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("amount", BigDecimal::from_str(&evt.amount).unwrap())
            .set("buyer", Hex(&evt.buyer).to_string());
    });
    events.moonbird_revenues.iter().for_each(|evt| {
        tables
            .create_row("moonbird_revenue", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("amount", BigDecimal::from_str(&evt.amount).unwrap())
            .set("beneficiary", Hex(&evt.beneficiary).to_string())
            .set("num_purchased", BigDecimal::from_str(&evt.num_purchased).unwrap());
    });
    events.moonbird_role_admin_changeds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_role_admin_changed", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("new_admin_role", Hex(&evt.new_admin_role).to_string())
            .set("previous_admin_role", Hex(&evt.previous_admin_role).to_string())
            .set("role", Hex(&evt.role).to_string());
    });
    events.moonbird_role_granteds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_role_granted", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("account", Hex(&evt.account).to_string())
            .set("role", Hex(&evt.role).to_string())
            .set("sender", Hex(&evt.sender).to_string());
    });
    events.moonbird_role_revokeds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_role_revoked", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("account", Hex(&evt.account).to_string())
            .set("role", Hex(&evt.role).to_string())
            .set("sender", Hex(&evt.sender).to_string());
    });
    events.moonbird_transfers.iter().for_each(|evt| {
        tables
            .create_row("moonbird_transfer", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("from", Hex(&evt.from).to_string())
            .set("to", Hex(&evt.to).to_string())
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_unnesteds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_unnested", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_unpauseds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_unpaused", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("account", Hex(&evt.account).to_string());
    });
}
fn db_bayc_out(events: &contract::Events, tables: &mut DatabaseChangeTables) {
    // Loop over all the abis events to create table changes
    events.bayc_approvals.iter().for_each(|evt| {
        tables
            .create_row("bayc_approval", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("approved", Hex(&evt.approved).to_string())
            .set("owner", Hex(&evt.owner).to_string())
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.bayc_approval_for_alls.iter().for_each(|evt| {
        tables
            .create_row("bayc_approval_for_all", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("approved", evt.approved)
            .set("operator", Hex(&evt.operator).to_string())
            .set("owner", Hex(&evt.owner).to_string());
    });
    events.bayc_ownership_transferreds.iter().for_each(|evt| {
        tables
            .create_row("bayc_ownership_transferred", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("new_owner", Hex(&evt.new_owner).to_string())
            .set("previous_owner", Hex(&evt.previous_owner).to_string());
    });
    events.bayc_transfers.iter().for_each(|evt| {
        tables
            .create_row("bayc_transfer", [("evt_tx_hash", evt.evt_tx_hash.to_string()),("evt_index", evt.evt_index.to_string())])
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("from", Hex(&evt.from).to_string())
            .set("to", Hex(&evt.to).to_string())
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
}


fn graph_moonbird_out(events: &contract::Events, tables: &mut EntityChangesTables) {
    // Loop over all the abis events to create table changes
    events.moonbird_approvals.iter().for_each(|evt| {
        tables
            .create_row("moonbird_approval", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("approved", Hex(&evt.approved).to_string())
            .set("owner", Hex(&evt.owner).to_string())
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_approval_for_alls.iter().for_each(|evt| {
        tables
            .create_row("moonbird_approval_for_all", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("approved", evt.approved)
            .set("operator", Hex(&evt.operator).to_string())
            .set("owner", Hex(&evt.owner).to_string());
    });
    events.moonbird_expelleds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_expelled", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_nesteds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_nested", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_ownership_transferreds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_ownership_transferred", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("new_owner", Hex(&evt.new_owner).to_string())
            .set("previous_owner", Hex(&evt.previous_owner).to_string());
    });
    events.moonbird_pauseds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_paused", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("account", Hex(&evt.account).to_string());
    });
    events.moonbird_refunds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_refund", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("amount", BigDecimal::from_str(&evt.amount).unwrap())
            .set("buyer", Hex(&evt.buyer).to_string());
    });
    events.moonbird_revenues.iter().for_each(|evt| {
        tables
            .create_row("moonbird_revenue", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("amount", BigDecimal::from_str(&evt.amount).unwrap())
            .set("beneficiary", Hex(&evt.beneficiary).to_string())
            .set("num_purchased", BigDecimal::from_str(&evt.num_purchased).unwrap());
    });
    events.moonbird_role_admin_changeds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_role_admin_changed", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("new_admin_role", Hex(&evt.new_admin_role).to_string())
            .set("previous_admin_role", Hex(&evt.previous_admin_role).to_string())
            .set("role", Hex(&evt.role).to_string());
    });
    events.moonbird_role_granteds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_role_granted", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("account", Hex(&evt.account).to_string())
            .set("role", Hex(&evt.role).to_string())
            .set("sender", Hex(&evt.sender).to_string());
    });
    events.moonbird_role_revokeds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_role_revoked", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("account", Hex(&evt.account).to_string())
            .set("role", Hex(&evt.role).to_string())
            .set("sender", Hex(&evt.sender).to_string());
    });
    events.moonbird_transfers.iter().for_each(|evt| {
        tables
            .create_row("moonbird_transfer", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("from", Hex(&evt.from).to_string())
            .set("to", Hex(&evt.to).to_string())
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_unnesteds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_unnested", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.moonbird_unpauseds.iter().for_each(|evt| {
        tables
            .create_row("moonbird_unpaused", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("account", Hex(&evt.account).to_string());
    });
}
fn graph_bayc_out(events: &contract::Events, tables: &mut EntityChangesTables) {
    // Loop over all the abis events to create table changes
    events.bayc_approvals.iter().for_each(|evt| {
        tables
            .create_row("bayc_approval", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("approved", Hex(&evt.approved).to_string())
            .set("owner", Hex(&evt.owner).to_string())
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
    events.bayc_approval_for_alls.iter().for_each(|evt| {
        tables
            .create_row("bayc_approval_for_all", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("approved", evt.approved)
            .set("operator", Hex(&evt.operator).to_string())
            .set("owner", Hex(&evt.owner).to_string());
    });
    events.bayc_ownership_transferreds.iter().for_each(|evt| {
        tables
            .create_row("bayc_ownership_transferred", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("new_owner", Hex(&evt.new_owner).to_string())
            .set("previous_owner", Hex(&evt.previous_owner).to_string());
    });
    events.bayc_transfers.iter().for_each(|evt| {
        tables
            .create_row("bayc_transfer", format!("{}-{}", evt.evt_tx_hash, evt.evt_index))
            .set("evt_tx_hash", &evt.evt_tx_hash)
            .set("evt_index", evt.evt_index)
            .set("evt_block_time", evt.evt_block_time.as_ref().unwrap())
            .set("evt_block_number", evt.evt_block_number)
            .set("from", Hex(&evt.from).to_string())
            .set("to", Hex(&evt.to).to_string())
            .set("token_id", BigDecimal::from_str(&evt.token_id).unwrap());
    });
}

#[substreams::handlers::map]
fn map_events(blk: eth::Block) -> Result<contract::Events, substreams::errors::Error> {
    let mut events = contract::Events::default();
    map_moonbird_events(&blk, &mut events);
    map_bayc_events(&blk, &mut events);
    Ok(events)
}

#[substreams::handlers::map]
fn db_out(events: contract::Events) -> Result<DatabaseChanges, substreams::errors::Error> {
    // Initialize Database Changes container
    let mut tables = DatabaseChangeTables::new();
    db_moonbird_out(&events, &mut tables);
    db_bayc_out(&events, &mut tables);
    Ok(tables.to_database_changes())
}

#[substreams::handlers::map]
fn graph_out(events: contract::Events) -> Result<EntityChanges, substreams::errors::Error> {
    // Initialize Database Changes container
    let mut tables = EntityChangesTables::new();
    graph_moonbird_out(&events, &mut tables);
    graph_bayc_out(&events, &mut tables);
    Ok(tables.to_entity_changes())
}
