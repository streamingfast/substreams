#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Transfers {
    #[prost(message, repeated, tag="1")]
    pub transfers: ::prost::alloc::vec::Vec<Transfer>,
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Transfer {
    #[prost(bytes="vec", tag="1")]
    pub from: ::prost::alloc::vec::Vec<u8>,
    #[prost(bytes="vec", tag="2")]
    pub to: ::prost::alloc::vec::Vec<u8>,
    #[prost(uint64, tag="3")]
    pub token_id: u64,
    #[prost(bytes="vec", tag="4")]
    pub trx_hash: ::prost::alloc::vec::Vec<u8>,
    #[prost(uint64, tag="5")]
    pub ordinal: u64,
}
