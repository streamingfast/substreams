use crate::Block;
use std::str::FromStr;

#[allow(dead_code)]
#[path = "./sf.substreams.v1.test.rs"]
pub mod test;

impl Into<String> for Block {
    fn into(self) -> String {
        format!("{}:{}", self.id, self.number)
    }
}

impl From<String> for Block {
    fn from(block_as_string: String) -> Self {
        let values: Vec<&str> = block_as_string.split(":").collect();
        println!("{:?}", values);
        if values.len() != 3 {
            return Self {
                id: "default".to_string(),
                number: 1,
            };
        }
        Self {
            id: values[0].to_string(),
            number: u64::from_str(values[1]).unwrap(),
        }
    }
}
