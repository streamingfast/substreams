use crate::externs;
use crate::memory::memory;

pub fn eth_call(input: Vec<u8>) -> Vec<u8> {
    unsafe {
        let rpc_response_ptr = memory::alloc(8);
        externs::rpc::eth_call(input.as_ptr(), input.len() as u32, rpc_response_ptr);
        return memory::get_output_data(rpc_response_ptr);
    }
}
