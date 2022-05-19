use proc_macro::TokenStream;

mod handler;

#[proc_macro_attribute]
pub fn handler(args: TokenStream, item: TokenStream) -> TokenStream {
    handler::main(args, item)
}
