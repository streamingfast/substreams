use proc_macro::TokenStream;
use quote::{quote};
use syn::{parse_macro_input, DeriveInput, Ident};

mod handler;


#[proc_macro_attribute]
#[cfg(not(test))] // Work around for rust-lang/rust#62127
pub fn handler(args: TokenStream, item: TokenStream) -> TokenStream {
    handler::main(args, item)
}
