// @generated
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Block {
    #[prost(string, tag="1")]
    pub id: ::prost::alloc::string::String,
    #[prost(uint64, tag="2")]
    pub number: u64,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct MapResult {
    #[prost(uint64, tag="1")]
    pub block_number: u64,
    #[prost(string, tag="2")]
    pub block_hash: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Boolean {
    #[prost(bool, tag="1")]
    pub result: bool,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Array {
    #[prost(string, repeated, tag="1")]
    pub result: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
// @@protoc_insertion_point(module)
