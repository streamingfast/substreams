//! Global allocator selection based on Cargo features.
//!
//! Only one allocator feature should be enabled at a time.
//! If no feature is enabled, the default dlmalloc is used.

/// Returns a string identifying which allocator is active
pub const fn allocator_name() -> &'static str {
    #[cfg(feature = "alloc-talc")]
    { "talc" }
    #[cfg(feature = "alloc-rlsf")]
    { "rlsf" }
    #[cfg(feature = "alloc-lol")]
    { "lol_alloc" }
    #[cfg(feature = "alloc-mini")]
    { "mini_alloc" }
    #[cfg(not(any(
        feature = "alloc-talc",
        feature = "alloc-rlsf",
        feature = "alloc-lol",
        feature = "alloc-mini"
    )))]
    { "default" }
}

#[cfg(all(target_arch = "wasm32", feature = "alloc-talc"))]
mod talc_allocator {
    use talc::{Talc, TalckWasm};

    #[global_allocator]
    static ALLOCATOR: TalckWasm = Talc::new(unsafe { talc::WasmHandler::new() }).lock();
}

#[cfg(all(target_arch = "wasm32", feature = "alloc-rlsf"))]
mod rlsf_allocator {
    use rlsf::SmallGlobalTlsf;

    #[global_allocator]
    static ALLOCATOR: SmallGlobalTlsf = SmallGlobalTlsf::new();
}

#[cfg(all(target_arch = "wasm32", feature = "alloc-lol"))]
mod lol_allocator {
    use lol_alloc::LeakingPageAllocator;

    #[global_allocator]
    static ALLOCATOR: LeakingPageAllocator = LeakingPageAllocator;
}

#[cfg(all(target_arch = "wasm32", feature = "alloc-mini"))]
mod mini_allocator {
    #[global_allocator]
    static ALLOCATOR: mini_alloc::MiniAlloc = mini_alloc::MiniAlloc::INIT;
}
