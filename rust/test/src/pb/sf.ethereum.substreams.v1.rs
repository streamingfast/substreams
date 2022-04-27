#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RpcCalls {
    #[prost(message, repeated, tag="1")]
    pub calls: ::prost::alloc::vec::Vec<RpcCall>,
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RpcCall {
    #[prost(bytes="vec", tag="1")]
    pub to_addr: ::prost::alloc::vec::Vec<u8>,
    #[prost(bytes="vec", tag="2")]
    pub method_signature: ::prost::alloc::vec::Vec<u8>,
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RpcResponses {
    #[prost(message, repeated, tag="1")]
    pub responses: ::prost::alloc::vec::Vec<RpcResponse>,
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RpcResponse {
    #[prost(bytes="vec", tag="1")]
    pub raw: ::prost::alloc::vec::Vec<u8>,
    #[prost(bool, tag="2")]
    pub failed: bool,
}
