//! Error implementation for Substreams.
//!
//! This crate implements Substreams error that you can
//! return in your Substreams handler
//!

use thiserror::Error;

#[derive(Error, Debug)]
pub enum Error {
    #[error("unexpected error: `{0}`")]
    Unexpected(String),
}
