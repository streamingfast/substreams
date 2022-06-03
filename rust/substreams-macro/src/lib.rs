use proc_macro::TokenStream;

mod config;
mod errors;
mod handler;
mod store;

#[proc_macro_attribute]
pub fn map(args: TokenStream, item: TokenStream) -> TokenStream {
    return handler::main(args, item, config::ModuleType::Map);
}

#[proc_macro_attribute]
pub fn store(args: TokenStream, item: TokenStream) -> TokenStream {
    return handler::main(args, item, config::ModuleType::Store);
}

#[proc_macro_derive(StoreWriter)]
pub fn derive(input: TokenStream) -> TokenStream {
    store::main(input)
}
