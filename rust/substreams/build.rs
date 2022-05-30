use std::{env, io::Result, path::Path};
fn main() -> Result<()> {
    let out_dir = env::var("CARGO_MANIFEST_DIR").unwrap();
    let path = Path::new(&out_dir).join("..").join("..").join("proto");
    println!("Proto path {:?}", &path);

    println!("cargo:rerun-if-changed=../../proto");

    let mut prost_build = prost_build::Config::new();
    prost_build.out_dir("./src/pb");
    prost_build.compile_protos(
        &["sf/substreams/v1/substreams.proto"],
        &[
            // When doing `cargo publish`, the actual build location will be different than when doing
            // 'cargo build' and we will be one directly deeper. So we accomated both cases by specifying
            // two include paths, first one resolved correctly when doing `cargo build`, second one does
            // resolve correctly when doing `cargo publish`.
            "../../proto",
            "../../../proto",
        ],
    )
}
