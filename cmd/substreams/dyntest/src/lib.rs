use wasmedge_bindgen::*;
use wasmedge_bindgen_macro::*;

#[wasmedge_bindgen]
pub fn sf_mycustomer_v1_eth_transfers(v: Vec<u8>) -> Result<Vec<u8>, String> {
    // TODO: decode `v` as our proto model
    // TODO: encode an output as another proto model
    // Integrate calls to `kv_get()` or something
    // that hits the HOST VM (in Go)
    return Ok(v);
}
