pub mod memory {
    use std::convert::TryInto;
    use std::slice;

    /// Allocate memory into the module's linear memory
    /// and return the offset to the start of the block.
    #[no_mangle]
    pub fn alloc(len: usize) -> *mut u8 {
        // create a new mutable buffer with capacity `len`
        let mut buf = Vec::with_capacity(len);
        // take a mutable pointer to the buffer
        let ptr = buf.as_mut_ptr();
        // take ownership of the memory block and
        // ensure the its destructor is not
        // called when the object goes out of scope
        // at the end of the function
        std::mem::forget(buf);
        // return the pointer so the runtime
        // can write data at this offset
        ptr
    }

    #[no_mangle]
    #[allow(clippy::missing_safety_doc)]
    pub unsafe fn dealloc(ptr: *mut u8, size: usize) {
        let data = Vec::from_raw_parts(ptr, size, size);
        std::mem::drop(data);
    }

    pub(crate) fn read_u32_from_heap(output_ptr: *mut u8, len: usize) -> u32 {
        unsafe {
            let value_bytes = slice::from_raw_parts(output_ptr, len);
            let value_raw_bytes: [u8; 4] = value_bytes.try_into().expect("error reading raw bytes");
            return u32::from_le_bytes(value_raw_bytes);
        }
    }

    pub(crate) fn get_output_data(output_ptr: *mut u8) -> Vec<u8> {
        unsafe {
            let value_ptr: u32 = read_u32_from_heap(output_ptr, 4);
            let value_len: u32 = read_u32_from_heap(output_ptr.add(4), 4);

            return Vec::from_raw_parts(
                value_ptr as *mut u8,
                value_len as usize,
                value_len as usize,
            );
        }
    }
}
