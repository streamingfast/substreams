use anyhow::{Ok, Result};
use substreams_ethereum::Abigen;

fn main() -> Result<(), anyhow::Error> {
    Abigen::new("Contract", "abi/contract.abi.json")?
        .generate()?
        .write_to_file("src/abi/contract.rs")?;

    Ok(())
}
