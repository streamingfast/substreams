// @generated
// Simple output for benchmarks that just count things
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct BenchOutput {
    #[prost(uint64, tag = "1")]
    pub count: u64,
    #[prost(string, tag = "2")]
    pub allocator_name: ::prost::alloc::string::String,
}

// Large output with structured data for mixed allocation patterns
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct LargeOutput {
    #[prost(message, repeated, tag = "1")]
    pub transactions: ::prost::alloc::vec::Vec<TxData>,
}

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct TxData {
    #[prost(string, tag = "1")]
    pub hash: ::prost::alloc::string::String,
    #[prost(string, tag = "2")]
    pub from: ::prost::alloc::string::String,
    #[prost(string, tag = "3")]
    pub to: ::prost::alloc::string::String,
    #[prost(message, repeated, tag = "4")]
    pub calls: ::prost::alloc::vec::Vec<CallData>,
}

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CallData {
    #[prost(uint64, tag = "1")]
    pub index: u64,
    #[prost(uint64, tag = "2")]
    pub input_size: u64,
    #[prost(uint64, tag = "3")]
    pub return_size: u64,
}
