#[link(wasm_import_module = "env")]
extern "C" {
    pub fn output(ptr: *const u8, len: u32);
    pub fn register_panic(
        msg_ptr: *const u8,
        msg_len: u32,
        file_ptr: *const u8,
        file_len: u32,
        line: u32,
        column: u32,
    );
}

#[link(wasm_import_module = "logger")]
extern "C" {
    pub fn debug(ptr: *const u8, len: usize);
    pub fn info(ptr: *const u8, len: usize);
}

pub mod state {
    #[link(wasm_import_module = "state")]
    extern "C" {
        pub fn get_first(store_idx: u32, key_ptr: *const u8, key_len: u32, output_ptr: u32) -> u32;
        pub fn get_last(store_idx: u32, key_ptr: *const u8, key_len: u32, output_ptr: u32) -> u32;
        pub fn get_at(
            store_idx: u32,
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            output_ptr: u32,
        ) -> u32;

        pub fn set(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );

        pub fn sum_big_int(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
    }
}
