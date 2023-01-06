mod pb;

#[allow(unused_imports)]
use wasmedge_bindgen::*;
use wasmedge_bindgen_macro::*;
use crate::pb::custom::{Request, Response};
use prost::Message;
use std::error::Error;

#[wasmedge_bindgen]
pub fn sf_mycustomer_v1_eth_transfers(v: Vec<u8>) -> Result<Vec<u8>, Error> {
    let req: Request = ::prost::Message::decode(&v[..])?;

    // Integrate calls to `kv_get()` or something
    // that hits the HOST VM (in Go)

    let resp = Response{
        output: format!("{}{}", req.input, req.input),
    };

    let mut buf = Vec::new();
    let encoded_len = resp.encoded_len();
    buf.reserve(encoded_len);
    match resp.encode(&mut buf) {
        Ok(_) => Ok(buf),
        Err(e) => Err(e),
    }
}
