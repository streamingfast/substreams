#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RpcCalls {
    #[prost(message, repeated, tag="1")]
    pub calls: ::std::vec::Vec<RpcCall>,
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RpcCall {
    #[prost(bytes, tag="1")]
    pub to_addr: std::vec::Vec<u8>,
    #[prost(bytes, tag="2")]
    pub method_signature: std::vec::Vec<u8>,
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RpcResponses {
    #[prost(message, repeated, tag="1")]
    pub responses: ::std::vec::Vec<RpcResponse>,
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RpcResponse {
    #[prost(bytes, tag="1")]
    pub raw: std::vec::Vec<u8>,
    #[prost(bool, tag="2")]
    pub failed: bool,
}
