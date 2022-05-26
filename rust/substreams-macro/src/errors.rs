use thiserror::Error;

#[derive(Error, Debug)]
pub enum SubstreamMacroError {
    #[error("unknown input type")]
    UnknownInputType(String),
}