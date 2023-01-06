mod pb;

#[allow(unused_imports)]
use wasmedge_bindgen::*;
use wasmedge_bindgen_macro::*;
use crate::pb::custom::{Request, Response};
use prost::Message;

#[wasmedge_bindgen]
pub fn sf_mycustomer_v1_eth_transfers(v: Vec<u8>) -> Vec<u8> {
    let req = Request::decode(&v[..]).expect("Failed to decode");

    // Integrate calls to `kv_get()` or something
    // that hits the HOST VM (in Go)

    let resp = Response {
        output: format!("{}{}", req.input, req.input),
    };

    resp.encode_to_vec()
}
