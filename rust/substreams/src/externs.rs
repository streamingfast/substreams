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
    pub fn println(ptr: *const u8, len: usize);
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
        pub fn set_if_not_exists(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
        pub fn append(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
        pub fn delete_prefix(ord: i64, prefix_ptr: *const u8, prefix_len: u32);
        pub fn add_bigint(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
        pub fn add_int64(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value: i64,
        );
        pub fn add_float64(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value: f64,
        );
        pub fn add_bigfloat(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
        pub fn set_min_int64(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value: i64,
        );
        pub fn set_min_bigint(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
        pub fn set_min_float64(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value: f64,
        );
        pub fn set_min_bigfloat(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
        pub fn set_max_int64(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value: i64,
        );
        pub fn set_max_bigint(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
        pub fn set_max_float64(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value: f64,
        );
        pub fn set_max_bigfloat(
            ord: i64,
            key_ptr: *const u8,
            key_len: u32,
            value_ptr: *const u8,
            value_len: u32,
        );
    }
}