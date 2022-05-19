use std::error::Error;
use std::fmt;

#[derive(Debug, Clone)]
pub struct SubstreamError {
    message: String
}

impl SubstreamError {
    pub fn new(msg: &str) -> SubstreamError {
        SubstreamError{message: msg.to_string()}
    }
}

impl fmt::Display for SubstreamError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f,"{}",self.message)
    }
}

impl Error for SubstreamError {
    fn description(&self) -> &str {
        &self.message
    }
}

