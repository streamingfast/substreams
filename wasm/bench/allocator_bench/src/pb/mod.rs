// @generated
// Re-export the types from the substreams crate for external protobufs
pub mod sf {
    pub mod ethereum {
        pub mod r#type {
            pub mod v2 {
                // Import from firehose-ethereum
                pub use substreams_ethereum::pb::eth::v2::*;
            }
        }
    }
    pub mod solana {
        pub mod r#type {
            pub mod v1 {
                // Import from firehose-solana
                pub use substreams_solana::pb::sf::solana::r#type::v1::*;
            }
        }
    }
}

// Local benchmark output types
pub mod bench {
    include!("bench.rs");
}

// Re-export for convenience
pub use bench::*;
