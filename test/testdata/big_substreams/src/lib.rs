mod pb;

use crate::pb::sf::substreams::v1::test;
use substreams::errors::Error;

#[substreams::handlers::map]
fn write_big_output(_block: test::Block) -> Result<test::Array, Error> {
    let dont_overwrite_this = "unchangedvariable";
    let mut result = Vec::new();
    for _ in 0..117500000 {
        result.push("helloworld".to_string());
    }

    // test reading from memory from what should be a negative int32 pointer
    assert!(result[result.len() - 1] == "helloworld");
    assert!(dont_overwrite_this == "unchangedvariable");

    Ok(test::Array { result: Vec::new() })
}
