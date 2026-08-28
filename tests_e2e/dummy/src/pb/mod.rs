// @generated
pub mod sf {
    pub mod acme {
        pub mod r#type {
            // @@protoc_insertion_point(attribute:sf.acme.type.v1)
            pub mod v1 {
                include!("sf.acme.type.v1.rs");
                // @@protoc_insertion_point(sf.acme.type.v1)
            }
        }
    }
}
// @@protoc_insertion_point(attribute:schema)
pub mod schema {
    include!("schema.rs");
    // @@protoc_insertion_point(schema)
}
pub mod test {
    // @@protoc_insertion_point(attribute:test.clickhouse)
    pub mod clickhouse {
        include!("test.clickhouse.rs");
        // @@protoc_insertion_point(test.clickhouse)
    }
    // @@protoc_insertion_point(attribute:test.output)
    pub mod output {
        include!("test.output.rs");
        // @@protoc_insertion_point(test.output)
    }
}
