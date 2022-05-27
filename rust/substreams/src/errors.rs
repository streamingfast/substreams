use thiserror::Error;

#[derive(Error, Debug)]
pub enum Error {
    #[error("unexpected error: `{0}`")]
    Unexpected(String),
}
