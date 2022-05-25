use proc_macro::TokenStream;
use quote::{quote};
use syn::{parse_macro_input, DeriveInput};

pub(crate) fn main(input: TokenStream) -> TokenStream {
    let input = parse_macro_input!(input as DeriveInput);
    let name = input.ident;

    let tokens = quote! {
        impl #name {
            pub fn new() -> #name { #name{} }
            pub fn delete_prefix(&self, ord: i64, prefix: &String) {
                state::delete_prefix(ord, prefix);
            }
        }
    };
    proc_macro::TokenStream::from(tokens)
}