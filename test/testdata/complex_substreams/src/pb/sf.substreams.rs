// @generated
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FieldOptions {
    /// this option informs the `substreams pack` command that it should treat the corresponding manifest value as a path to a file, putting its content as bytes in this field. 
    /// must be applied to a `bytes` or `string` field
    #[prost(bool, tag="1")]
    pub load_from_file: bool,
    /// this option informs the `substreams pack` command that it should treat the corresponding manifest value as a path to a folder, zipping its content and putting the zip content as bytes in this field.
    /// must be applied to a `bytes` field
    #[prost(bool, tag="2")]
    pub zip_from_folder: bool,
}
// @@protoc_insertion_point(module)
