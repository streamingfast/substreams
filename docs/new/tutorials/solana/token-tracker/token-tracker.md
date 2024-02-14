The Solana Token Tracker Substreams allows you to extract transfers from Solana Token Programs. You can simply provide the address of the token you want to track as an input to the Substreams.

## Before You Begin

The Solana Token Tracker Substreams requires medium to advanced Substreams knowledge. If this is the first time you are using Substreams, make sure you:

- Read the [Develop Substreams](../../../develop/develop.md) section, which will teach you the basics of the developing Substreams modules.
- Complete the [Explore Solana](../explore-solana/explore-solana.md) tutorial, which will assist you in understanding the main pieces of the Solana Substreams.

If you already have the required knowledge, clone the [Solana Token Tracker GitHub repository](https://github.com/streamingfast/solana-token-tracker). You will go through the code in the following steps.

## Inspect the Project

The Substreams has only one module: `map_solana_token_events`, as you can check in the Substreams manifest (`substreams.yaml`):

```yaml
modules:
  - name: map_solana_token_events 
    kind: map
    initialBlock: 158558168
    inputs:
      - params: string
      - source: sf.solana.type.v1.Block
    output:
      type: proto:solana_token_tracker.types.v1.Output
params:
  map_solana_token_events: "token_contract=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v&token_decimals=6"
```

The module receives two inputs (defined in the `intputs` section of the YAML):
- A string containing a couple of parameters: this parameter is defined in the `params` section of the YAML, and defines the token that you want to extract data from: `token_contract=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v&token_decimals=6`
`token_contract=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v` is the address of the USDC contract in Solana mainnet and `token_decimals=6` is the number of decimals used the USDC token.
- A raw Solana block.

You can update the `token_contract` parameter to track any token of your choice. You can also use the `-p` option in the Substreams GUI to dynamically override the parameters of the Substreams.

## Run the Substreams

You can run the Substreams by using the Substreams CLI. As specified in the manifest by default, the USDC data will be retrieved.

```bash
substreams gui ./substreams.yaml map_solana_token_events -e mainnet.sol.streamingfast.io:443  --start-block 158558168 --stop-block +1
```

You can also override the parameters of the manifest by using the `-p` option of the CLI. For example, if you want to track the transfer of the USDT token (`Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB`):

```bash
substreams gui ./substreams.yaml map_solana_token_events -e mainnet.sol.streamingfast.io:443  --start-block 158558168 --stop-block +1 -p map_solana_token_events="token_contract=Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB&token_decimals=6"
```

## Inspect the Code

- Open the `lib.rs` file, which contains the code for the `map_solana_token_events` module.
The function receives two parameters: the raw Solana block object and the parameters provided in the Substreams manifest.

- The `parse_parameters` function converts the parameters string passed to the module and converts it into a `TokenParams` object.
This object contains two fields: `token_contract` (representing the address of the token to track) and `token_decimals` (representing the number of decimals used in the token).

```rust
pub fn map_solana_token_events(params: String, block: Block) -> Result<Output, Error> {
    let parameters = parse_parameters(params)?;

    // ...
}
```

- Then, you iterate over all the transactions

```rust
pub fn map_solana_token_events(params: String, block: Block) -> Result<Output, Error> {
    let parameters = parse_parameters(params)?;

    let mut output = Output::default(); // 1.
    let timestamp = block.block_time.as_ref().unwrap().timestamp;

    for confirmed_trx in block.transactions_owned() { // 2.
        let accounts = confirmed_trx.resolved_accounts_as_strings(); // 3.

        if let Some(trx) = confirmed_trx.transaction { // 4.
            let trx_hash = bs58::encode(&trx.signatures[0]).into_string();
            let msg = trx.message.unwrap(); // 5.
            let meta = confirmed_trx.meta.as_ref().unwrap(); // 6.

            for (i, compiled_instruction) in msg.instructions.iter().enumerate() { // 7.
                utils::process_compiled_instruction( // 8.
                    &mut output,
                    timestamp,
                    &trx_hash,
                    meta,
                    i as u32,
                    compiled_instruction,
                    &accounts,
                    &parameters
                );
            }
        }
    }

    Ok(output) // 9.
}
```
1. Create an `Output` object, which is the container of all the events extracted.
2. Iterate over the confirmed transactions of the block.
3. Get the accounts of the transaction. The `resolved_accounts()` method contains also accounts stored in the [Address Lookup Tables](https://docs.solana.com/developing/lookup-tables).
4. _Unwrap_ the transaction if it is available.
5. _Unwrap_ the transaction message.
6. _Unwrap_ the transaction metadata.
7. Iterave over the instructions contained within the transaction.
8. For every instruction, call the `process_compiled_instruction(...)` function to process the instruction further.

- The `process_compiled_instruction(...)` function is defined in the `util.rs` file.

```rust
pub fn process_compiled_instruction(
    output: &mut Output,
    timestamp: i64,
    trx_hash: &String,
    meta: &TransactionStatusMeta,
    inst_index: u32,
    inst: &CompiledInstruction,
    accounts: &Vec<String>,
    parameters: &TokenParams
) {
    let instruction_program_account = &accounts[inst.program_id_index as usize]; // 1.

    if instruction_program_account == constants::TOKEN_PROGRAM { // 2.
        match process_token_instruction(trx_hash, timestamp, &inst.data, &inst.accounts, meta, accounts, output, parameters) {
            Err(err) => {
                panic!(
                    "trx_hash {} top level transaction without inner instructions: {}",
                    trx_hash, err
                );
            }
            Ok(()) => {}
        }

    }

    process_inner_instructions(output, inst_index, meta, accounts, trx_hash, timestamp, parameters); // 3.
}
```
1. The `instruction.program_id_index` indicates the position of the program account in the accounts array. For example, if `program_index_id = 5`, it means that the program account will be at position number 5 in the `accounts` array.
2. If the instruction account is the Token Program Account (i.e. `TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA`), this means that the instruction executed in the transaction has been produced by the Token Program. Therefore, you process the instruction further by calling the `process_token_instruction(...)` function to extract token-related information, such as transfers or mints.
3. Every top-level instruction holds inner instructions. If the top-level instruction is not from the Token Program, you check if any Inner Instruction is from the Token Program by calling the `process_inner_instructions(...)` function.

- A top-level instruction could hold a Token Program instruction within its inner instructions. The `process_inner_instructions(...)` checks if there are Token Program among the inner instructions of every top-level instruction.

```rust
pub fn process_inner_instructions(
    output: &mut Output,
    instruction_index: u32,
    meta: &TransactionStatusMeta,
    accounts: &Vec<String>,
    trx_hash: &String,
    timestamp: i64,
    parameters: &TokenParams,
) {
    meta.inner_instructions // 1.
        .iter()
        .filter(|inst| inst.index == instruction_index) // 2.
        .for_each(|inst| { // 3.
            inst.instructions
                .iter() // 4.
                .filter(|&inner_instruction| { // 5.
                    let instruction_program_account = &accounts[inner_instruction.program_id_index as usize];
                    instruction_program_account == constants::TOKEN_PROGRAM
                })
                .for_each(|inner_instruction| {
                    match process_token_instruction( // 6.
                        trx_hash,
                        timestamp,
                        &inner_instruction.data,
                        &inner_instruction.accounts,
                        meta,
                        accounts,
                        output,
                        parameters
                    ) {
                        Err(err) => {
                            panic!("trx_hash {} filtering inner instructions: {}", trx_hash, err)
                        }
                        Ok(()) => {}
                    }
                })
        });
}
```
1. The `TransactionStatusMeta` object holds an array with the inner instructions of the transaction (an array of `InnerTransactions` objects).
2. Because the inner instructions are at the transaction level (contained within the `TransactionStatusMeta`), you keep only the inner transactions belonging to the current top-level instruction. For this purpose, an index variable (`instruction_index`) is passed as a parameter.
Essentially, you are matching every top-level instruction with its corresponding `InnerTransactions` object. The filtering should only keep **one** `InnerTransactions` object, as every top-level instruction should only have one `InnerTransactions` object.
3. The `InnerTransactions` object is a just wrapper for the array of inner transactions. For every `InnerTransactions` object filtered (which should be **only one**), you actually extract the inner instructions.
4. You iterate over the array of inner instructions.
5. You only keep Token Program inner instructions.
6. You process every Token Program inner instruction found further by calling the `process_token_instruction(...)`.

Once you have identified all the Token Program instructions, the `proces_token_instruction(...)` function extracts transfer or mint data from these instructions. To easily extract data from a Token Program instruction, the Substreams relies on the `substreams-solana-program-instructions` Rust crate, which provides useful helper functions.

```rust
fn process_token_instruction(
    trx_hash: &String,
    timestamp: i64,
    data: &Vec<u8>,
    inst_accounts: &Vec<u8>,
    meta: &TransactionStatusMeta,
    accounts: &Vec<String>,
    output: &mut Output,
    parameters: &TokenParams,
) -> Result<(),Error> {
    match TokenInstruction::unpack(&data) { // 1.
        Err(err) => { // 2.
            substreams::log::info!("unpacking token instruction {:?}", err);
            return Err(anyhow::anyhow!("unpacking token instruction: {}", err));
        }
        Ok(instruction) => match instruction { // 3.
            TokenInstruction::Transfer { amount: amt }  => { // 4.
                let authority = &accounts[inst_accounts[2] as usize];
                if is_token_transfer(&meta.pre_token_balances, &authority, &parameters.token_contract) { // 5.
                    let source = &accounts[inst_accounts[0] as usize];
                    let destination = &accounts[inst_accounts[1] as usize];
                    output.transfers.push(Transfer { // 6.
                        trx_hash: trx_hash.to_owned(),
                        timestamp,
                        from: source.to_owned(),
                        to: destination.to_owned(),
                        amount: amount_to_decimals(amt as f64, parameters.token_decimals as f64),
                    });
                    return Ok(());
                }
            }

            // ...code omitted...
        }
    }
}
```
1. The `TokenInstruction::unpack(...)` function decodes the instruction and allows you to identify the action executed: `Transfer`, `TransferChecked`, `Mint`, or `Burn`.
2. Controlled way to handle errors from the `unpack(...)` function.
3. If there are no errors, then you can handle every action (`Transfer`, `Mint`...) differently.
4. Handle the `Transfer` instruction.
5. Call the `is_token_transfer(...)` to verify if the transfer is from the specified in the parameters of the Substreams module.
Note that you pass `parameters.token_contract` as a parameter to the function.
6. Create a new `Transfer` object from the Protobuf with the corresponding data. This object is added to the `Output` object and will be emitted as the output of the Substreams.

The code for other actions (`Mint`, `Burn`...) is analogous to the code of the `Transfer` instructions.
