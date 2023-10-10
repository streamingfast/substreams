use anyhow::{Ok, Result};
use regex::Regex;
use substreams_ethereum::Abigen;
use std::fs;
use std::fs::File;
use std::io::Write;

fn main() -> Result<(), anyhow::Error> {
    let contents = fs::read_to_string("abi/contract.abi.json")
        .expect("Should have been able to read the file");

    // sanitize fields and attributes starting with an underscore
    let regex = Regex::new(r#"("\w+"\s?:\s?")_(\w+")"#).unwrap();
    let sanitized_abi_file = regex.replace_all(contents.as_str(), "${1}u_${2}");

    // do not modify the original abi
    let mut file = File::create("/tmp/contract.abi.json")?;
    file.write_all(sanitized_abi_file.as_bytes())?;

    Abigen::new("Contract", "/tmp/contract.abi.json")?
        .generate()?
        .write_to_file("src/abi/contract.rs")?;

    Ok(())
}
