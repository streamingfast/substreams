// @generated
// @@protoc_insertion_point(attribute:schema)
pub mod schema {
    include!("schema.rs");
    // @@protoc_insertion_point(schema)
}
pub mod sf {
    pub mod ethereum {
        pub mod r#type {
            // @@protoc_insertion_point(attribute:sf.ethereum.type.v2)
            pub mod v2 {
                include!("sf.ethereum.type.v2.rs");
                // @@protoc_insertion_point(sf.ethereum.type.v2)
            }
        }
    }
    // @@protoc_insertion_point(attribute:sf.substreams)
    pub mod substreams {
        include!("sf.substreams.rs");
        // @@protoc_insertion_point(sf.substreams)
        pub mod sink {
            pub mod service {
                // @@protoc_insertion_point(attribute:sf.substreams.sink.service.v1)
                pub mod v1 {
                    include!("sf.substreams.sink.service.v1.rs");
                    // @@protoc_insertion_point(sf.substreams.sink.service.v1)
                }
            }
            pub mod sql {
                pub mod service {
                    // @@protoc_insertion_point(attribute:sf.substreams.sink.sql.service.v1)
                    pub mod v1 {
                        include!("sf.substreams.sink.sql.service.v1.rs");
                        // @@protoc_insertion_point(sf.substreams.sink.sql.service.v1)
                    }
                }
                // @@protoc_insertion_point(attribute:sf.substreams.sink.sql.v1)
                pub mod v1 {
                    include!("sf.substreams.sink.sql.v1.rs");
                    // @@protoc_insertion_point(sf.substreams.sink.sql.v1)
                }
            }
        }
        // @@protoc_insertion_point(attribute:sf.substreams.v1)
        pub mod v1 {
            include!("sf.substreams.v1.rs");
            // @@protoc_insertion_point(sf.substreams.v1)
        }
    }
}
