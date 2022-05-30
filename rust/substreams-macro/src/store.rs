use proc_macro::TokenStream;
use quote::{quote};
use syn::{parse_macro_input, DeriveInput};

pub(crate) fn main(input: TokenStream) -> TokenStream {
    let input = parse_macro_input!(input as DeriveInput);
    let name = input.ident;

    let tokens = quote! {
        impl #name {
            pub fn new() -> #name { #name{} }

            /// Allows you to delete a set of keys by prefix. Do not use this to delete
            /// individual keys if you want consistent highly performant parallelized operations.
            /// Rather, design key spaces where you can delete large number of keys in
            /// one swift using a meaningful prefix.
            pub fn delete_prefix(&self, ord: i64, prefix: &String) {
                state::delete_prefix(ord, prefix);
            }
        }
    };
    proc_macro::TokenStream::from(tokens)
}