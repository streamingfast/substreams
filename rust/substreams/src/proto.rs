use prost::{DecodeError, EncodeError};
use std::io::Cursor;

pub fn decode<T: std::default::Default + prost::Message>(buf: Vec<u8>) -> Result<T, DecodeError> {
    ::prost::Message::decode(&mut Cursor::new(&buf))
}

pub fn decode_ptr<T: std::default::Default + prost::Message>(
    ptr: *mut u8,
    size: usize,
) -> Result<T, DecodeError> {
    unsafe {
        let input_data = Vec::from_raw_parts(ptr, size, size);
        let obj = ::prost::Message::decode(&mut Cursor::new(&input_data));
        std::mem::forget(input_data); // otherwise tries to free that memory at the end and crashes

        obj
    }
}

pub fn encode<M: prost::Message>(msg: &M) -> Result<Vec<u8>, EncodeError> {
    let mut buf = Vec::new();

    let encoded_len = msg.encoded_len();
    buf.reserve(encoded_len);

    match msg.encode(&mut buf) {
        Ok(_) => Ok(buf),
        Err(e) => Err(e),
    }
}

pub fn encode_to_ptr<M: prost::Message>(msg: &M) -> Result<(*const u8, usize), EncodeError> {
    match encode(msg) {
        Ok(buffer) => Ok((buffer.as_ptr(), buffer.len())),
        Err(e) => Err(e),
    }
}
