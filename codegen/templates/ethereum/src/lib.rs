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

const BAYC_TRACKED_CONTRACT: [u8; 20] = hex!("bc4ca0eda7647a8ab7c2061c2e118a18a936f13d");

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
fn map_bayc_calls(blk: &eth::Block, calls: &mut contract::Calls) {
    calls.bayc_call_approves.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::Approve::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::Approve::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycApproveCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                to: decoded_call.to,
                                token_id: decoded_call.token_id.to_string(),
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_emergency_set_starting_index_blocks.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::EmergencySetStartingIndexBlock::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::EmergencySetStartingIndexBlock::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycEmergencySetStartingIndexBlockCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_flip_sale_states.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::FlipSaleState::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::FlipSaleState::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycFlipSaleStateCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_mint_apes.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::MintApe::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::MintApe::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycMintApeCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                number_of_tokens: decoded_call.number_of_tokens.to_string(),
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_renounce_ownerships.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::RenounceOwnership::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::RenounceOwnership::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycRenounceOwnershipCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_reserve_apes.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::ReserveApes::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::ReserveApes::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycReserveApesCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_safe_transfer_from_1s.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::SafeTransferFrom1::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::SafeTransferFrom1::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycSafeTransferFrom1Call {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                from: decoded_call.from,
                                to: decoded_call.to,
                                token_id: decoded_call.token_id.to_string(),
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_safe_transfer_from_2s.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::SafeTransferFrom2::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::SafeTransferFrom2::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycSafeTransferFrom2Call {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                from: decoded_call.from,
                                to: decoded_call.to,
                                token_id: decoded_call.token_id.to_string(),
                                u_data: decoded_call.u_data,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_set_approval_for_alls.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::SetApprovalForAll::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::SetApprovalForAll::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycSetApprovalForAllCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                approved: decoded_call.approved,
                                operator: decoded_call.operator,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_set_base_uris.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::SetBaseUri::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::SetBaseUri::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycSetBaseUriCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                base_uri: decoded_call.base_uri,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_set_provenance_hashes.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::SetProvenanceHash::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::SetProvenanceHash::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycSetProvenanceHashCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                provenance_hash: decoded_call.provenance_hash,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_set_reveal_timestamps.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::SetRevealTimestamp::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::SetRevealTimestamp::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycSetRevealTimestampCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                reveal_time_stamp: decoded_call.reveal_time_stamp.to_string(),
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_set_starting_indices.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::SetStartingIndex::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::SetStartingIndex::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycSetStartingIndexCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_transfer_froms.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::TransferFrom::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::TransferFrom::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycTransferFromCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                from: decoded_call.from,
                                to: decoded_call.to,
                                token_id: decoded_call.token_id.to_string(),
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_transfer_ownerships.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::TransferOwnership::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::TransferOwnership::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycTransferOwnershipCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                                new_owner: decoded_call.new_owner,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
    calls.bayc_call_withdraws.append(&mut blk
        .transactions()
        .flat_map(|tx| {
            tx.calls.iter()
                .filter(|call| call.address == BAYC_TRACKED_CONTRACT && abi::bayc_contract::functions::Withdraw::match_call(call))
                .filter_map(|call| {
                    match abi::bayc_contract::functions::Withdraw::decode(call) {
                        Ok(decoded_call) => {
                            Some(contract::BaycWithdrawCall {
                                call_tx_hash: Hex(&tx.hash).to_string(),
                                call_block_time: Some(blk.timestamp().to_owned()),
                                call_block_number: blk.number,
                                call_ordinal: call.begin_ordinal,
                                call_success: !call.status_reverted,
                            })
                        },
                        Err(_) => None,
                    }
                })
        })
        .collect());
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
fn db_bayc_calls_out(calls: &contract::Calls, tables: &mut DatabaseChangeTables) {
    // Loop over all the abis calls to create table changes
    calls.bayc_call_approves.iter().for_each(|call| {
        tables
            .create_row("bayc_call_approve", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("to", Hex(&call.to).to_string())
            .set("token_id", BigDecimal::from_str(&call.token_id).unwrap());
    });
    calls.bayc_call_emergency_set_starting_index_blocks.iter().for_each(|call| {
        tables
            .create_row("bayc_call_emergency_set_starting_index_block", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_flip_sale_states.iter().for_each(|call| {
        tables
            .create_row("bayc_call_flip_sale_state", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_mint_apes.iter().for_each(|call| {
        tables
            .create_row("bayc_call_mint_ape", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("number_of_tokens", BigDecimal::from_str(&call.number_of_tokens).unwrap());
    });
    calls.bayc_call_renounce_ownerships.iter().for_each(|call| {
        tables
            .create_row("bayc_call_renounce_ownership", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_reserve_apes.iter().for_each(|call| {
        tables
            .create_row("bayc_call_reserve_apes", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_safe_transfer_from_1s.iter().for_each(|call| {
        tables
            .create_row("bayc_call_safe_transfer_from1", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("from", Hex(&call.from).to_string())
            .set("to", Hex(&call.to).to_string())
            .set("token_id", BigDecimal::from_str(&call.token_id).unwrap());
    });
    calls.bayc_call_safe_transfer_from_2s.iter().for_each(|call| {
        tables
            .create_row("bayc_call_safe_transfer_from2", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("from", Hex(&call.from).to_string())
            .set("to", Hex(&call.to).to_string())
            .set("token_id", BigDecimal::from_str(&call.token_id).unwrap())
            .set("u_data", Hex(&call.u_data).to_string());
    });
    calls.bayc_call_set_approval_for_alls.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_approval_for_all", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("approved", call.approved)
            .set("operator", Hex(&call.operator).to_string());
    });
    calls.bayc_call_set_base_uris.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_base_uri", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("base_uri", &call.base_uri);
    });
    calls.bayc_call_set_provenance_hashes.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_provenance_hash", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("provenance_hash", &call.provenance_hash);
    });
    calls.bayc_call_set_reveal_timestamps.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_reveal_timestamp", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("reveal_time_stamp", BigDecimal::from_str(&call.reveal_time_stamp).unwrap());
    });
    calls.bayc_call_set_starting_indices.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_starting_index", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_transfer_froms.iter().for_each(|call| {
        tables
            .create_row("bayc_call_transfer_from", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("from", Hex(&call.from).to_string())
            .set("to", Hex(&call.to).to_string())
            .set("token_id", BigDecimal::from_str(&call.token_id).unwrap());
    });
    calls.bayc_call_transfer_ownerships.iter().for_each(|call| {
        tables
            .create_row("bayc_call_transfer_ownership", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("new_owner", Hex(&call.new_owner).to_string());
    });
    calls.bayc_call_withdraws.iter().for_each(|call| {
        tables
            .create_row("bayc_call_withdraw", [("call_tx_hash", call.call_tx_hash.to_string()),("call_ordinal", call.call_ordinal.to_string())])
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
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
fn graph_bayc_calls_out(calls: &contract::Calls, tables: &mut EntityChangesTables) {
    // Loop over all the abis calls to create table changes
    calls.bayc_call_approves.iter().for_each(|call| {
        tables
            .create_row("bayc_call_approve", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("to", Hex(&call.to).to_string())
            .set("token_id", BigDecimal::from_str(&call.token_id).unwrap());
    });
    calls.bayc_call_emergency_set_starting_index_blocks.iter().for_each(|call| {
        tables
            .create_row("bayc_call_emergency_set_starting_index_block", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_flip_sale_states.iter().for_each(|call| {
        tables
            .create_row("bayc_call_flip_sale_state", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_mint_apes.iter().for_each(|call| {
        tables
            .create_row("bayc_call_mint_ape", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("number_of_tokens", BigDecimal::from_str(&call.number_of_tokens).unwrap());
    });
    calls.bayc_call_renounce_ownerships.iter().for_each(|call| {
        tables
            .create_row("bayc_call_renounce_ownership", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_reserve_apes.iter().for_each(|call| {
        tables
            .create_row("bayc_call_reserve_apes", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_safe_transfer_from_1s.iter().for_each(|call| {
        tables
            .create_row("bayc_call_safe_transfer_from1", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("from", Hex(&call.from).to_string())
            .set("to", Hex(&call.to).to_string())
            .set("token_id", BigDecimal::from_str(&call.token_id).unwrap());
    });
    calls.bayc_call_safe_transfer_from_2s.iter().for_each(|call| {
        tables
            .create_row("bayc_call_safe_transfer_from2", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("from", Hex(&call.from).to_string())
            .set("to", Hex(&call.to).to_string())
            .set("token_id", BigDecimal::from_str(&call.token_id).unwrap())
            .set("u_data", Hex(&call.u_data).to_string());
    });
    calls.bayc_call_set_approval_for_alls.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_approval_for_all", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("approved", call.approved)
            .set("operator", Hex(&call.operator).to_string());
    });
    calls.bayc_call_set_base_uris.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_base_uri", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("base_uri", &call.base_uri);
    });
    calls.bayc_call_set_provenance_hashes.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_provenance_hash", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("provenance_hash", &call.provenance_hash);
    });
    calls.bayc_call_set_reveal_timestamps.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_reveal_timestamp", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("reveal_time_stamp", BigDecimal::from_str(&call.reveal_time_stamp).unwrap());
    });
    calls.bayc_call_set_starting_indices.iter().for_each(|call| {
        tables
            .create_row("bayc_call_set_starting_index", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
    calls.bayc_call_transfer_froms.iter().for_each(|call| {
        tables
            .create_row("bayc_call_transfer_from", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("from", Hex(&call.from).to_string())
            .set("to", Hex(&call.to).to_string())
            .set("token_id", BigDecimal::from_str(&call.token_id).unwrap());
    });
    calls.bayc_call_transfer_ownerships.iter().for_each(|call| {
        tables
            .create_row("bayc_call_transfer_ownership", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success)
            .set("new_owner", Hex(&call.new_owner).to_string());
    });
    calls.bayc_call_withdraws.iter().for_each(|call| {
        tables
            .create_row("bayc_call_withdraw", format!("{}-{}", call.call_tx_hash, call.call_ordinal))
            .set("call_tx_hash", &call.call_tx_hash)
            .set("call_ordinal", call.call_ordinal)
            .set("call_block_time", call.call_block_time.as_ref().unwrap())
            .set("call_block_number", call.call_block_number)
            .set("call_success", call.call_success);
    });
  }

#[substreams::handlers::map]
fn map_events(blk: eth::Block) -> Result<contract::Events, substreams::errors::Error> {
    let mut events = contract::Events::default();
    map_bayc_events(&blk, &mut events);
    Ok(events)
}

#[substreams::handlers::map]
fn db_out(events: contract::Events) -> Result<DatabaseChanges, substreams::errors::Error> {

    // Initialize Database Changes container
    let mut tables = DatabaseChangeTables::new();
    db_bayc_out(&events, &mut tables);
    db_bayc_calls_out(&calls, &mut tables);
    Ok(tables.to_database_changes())
}

#[substreams::handlers::map]
fn graph_out(events: contract::Events) -> Result<EntityChanges, substreams::errors::Error> {

    // Initialize Database Changes container
    let mut tables = EntityChangesTables::new();
    graph_bayc_out(&events, &mut tables);
    graph_bayc_calls_out(&calls, &mut tables);
    Ok(tables.to_entity_changes())
}
