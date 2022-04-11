#[derive(Clone, PartialEq, ::prost::Message)]
pub struct StoreDeltas {
    #[prost(message, repeated, tag="1")]
    pub deltas: ::std::vec::Vec<StoreDelta>,
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct StoreDelta {
    #[prost(enumeration="store_delta::Operation", tag="1")]
    pub operation: i32,
    #[prost(uint64, tag="2")]
    pub ordinal: u64,
    #[prost(string, tag="3")]
    pub key: std::string::String,
    #[prost(bytes, tag="4")]
    pub old_value: std::vec::Vec<u8>,
    #[prost(bytes, tag="5")]
    pub new_value: std::vec::Vec<u8>,
}
pub mod store_delta {
    #[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
    #[repr(i32)]
    pub enum Operation {
        Unset = 0,
        Create = 1,
        Update = 2,
        Delete = 3,
    }
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DatabaseChanges {
    #[prost(message, repeated, tag="1")]
    pub table_changes: ::std::vec::Vec<TableChange>,
}
//table.1
// create f.1 o.10 n.99
// update f.1 o.99 n.200
// update f.2 o.abc n.xyz
// update f.1 o.200 n.400

//table.1
// create f.1 o.10 n.400 f.2 o.abc n.xyz

//table.1
// update f.1 o.10 n.99
// update f.1 o.99 n.200
// update f.2 o.abc n.xyz
// update f.1 o.200 n.400

//table.1
// update f.1 o.10 n.400 f.2 o.abc n.xyz

#[derive(Clone, PartialEq, ::prost::Message)]
pub struct TableChange {
    #[prost(string, tag="1")]
    pub table: std::string::String,
    #[prost(string, tag="2")]
    pub pk: std::string::String,
    #[prost(uint64, tag="3")]
    pub block_num: u64,
    #[prost(uint64, tag="4")]
    pub ordinal: u64,
    #[prost(enumeration="table_change::Operation", tag="5")]
    pub operation: i32,
    #[prost(message, repeated, tag="6")]
    pub fields: ::std::vec::Vec<Field>,
}
pub mod table_change {
    #[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
    #[repr(i32)]
    pub enum Operation {
        Create = 0,
        Update = 1,
        Delete = 2,
    }
}
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Field {
    #[prost(string, tag="1")]
    pub name: std::string::String,
    #[prost(string, tag="2")]
    pub new_value: std::string::String,
    #[prost(string, tag="3")]
    pub old_value: std::string::String,
}
