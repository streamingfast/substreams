
use proc_macro::TokenStream;
use proc_macro2::{Span};
use quote::{quote, ToTokens, format_ident};
use syn::{parse::Parser, spanned::Spanned};

type AttributeArgs = syn::punctuated::Punctuated<syn::NestedMeta, syn::Token![,]>;

#[derive(Clone, Copy, PartialEq)]
enum ModuleType {
    Store,
    Map,
}

impl ModuleType {
    fn from_str(s: &str) -> Result<ModuleType, String> {
        match s {
            "store" => Ok(ModuleType::Store),
            "map" => Ok(ModuleType::Map),
            _ => Err(format!("No such module type `{}`. The modules types are `store` and `map`.", s)),
        }
    }
}

struct FinalConfiguration {
    module_type: ModuleType
}

struct Configuration {
    module_type: Option<ModuleType>
}


impl Configuration {
    fn new() -> Self {
        Configuration {
            module_type: None,
        }
    }

    fn set_module_type(&mut self, runtime: syn::Lit, span: Span) -> Result<(), syn::Error> {
        if self.module_type.is_some() {
            return Err(syn::Error::new(span, "`type` set multiple times."));
        }

        let runtime_str = parse_string(runtime, span, "type")?;
        let mod_type =
            ModuleType::from_str(&runtime_str).map_err(|err| syn::Error::new(span, err))?;
        self.module_type = Some(mod_type);
        Ok(())
    }

    fn build(&self) -> Result<FinalConfiguration, syn::Error> {
        let mod_type = self.module_type.unwrap_or(ModuleType::Map);
        Ok(FinalConfiguration {
            module_type: mod_type,
        })
    }
}

fn parse_string(int: syn::Lit, span: Span, field: &str) -> Result<String, syn::Error> {
    match int {
        syn::Lit::Str(s) => Ok(s.value()),
        syn::Lit::Verbatim(s) => Ok(s.to_string()),
        _ => Err(syn::Error::new(
            span,
            format!("Failed to parse value of `{}` as string.", field),
        )),
    }
}


fn build_config(
    args: AttributeArgs,
) -> Result<FinalConfiguration, syn::Error> {

    let mut config = Configuration::new();

    for arg in args {
        match arg {
            syn::NestedMeta::Meta(syn::Meta::NameValue(namevalue)) => {
                let ident = namevalue
                    .path
                    .get_ident()
                    .ok_or_else(|| {
                        syn::Error::new_spanned(&namevalue, "Must have specified ident")
                    })?
                    .to_string()
                    .to_lowercase();
                match ident.as_str() {
                    "type" => {
                        config.set_module_type(
                            namevalue.lit.clone(),
                            syn::spanned::Spanned::span(&namevalue.lit),
                        )?;
                    }
                    name => {
                        let msg = format!(
                            "Unknown attribute {} is specified; expected one of: `type`",
                            name,
                        );
                        return Err(syn::Error::new_spanned(namevalue, msg));
                    }
                }
            }
            other => {
                return Err(syn::Error::new_spanned(
                    other,
                    "Unknown attribute inside the macro",
                ));
            }
        }
    }
    config.build()
}

pub(crate) fn main(args: TokenStream, item: TokenStream) -> TokenStream {
    let original = item.clone();

    let config_result = AttributeArgs::parse_terminated.parse(args)
        .and_then(|args| build_config(args));

    let final_config = match config_result {
        Ok(f) => f,
        Err(e) => {
            return token_stream_with_error(original, e)
        }
    };
    let input = syn::parse_macro_input!(item as syn::ItemFn);

    let output_result = parse_func_output(&final_config, input.sig.output.clone());
    match output_result {
        Ok(_) => {}
        Err(e) => {
            return token_stream_with_error(original, e)
        }
    }

    let mut args : Vec<proc_macro2::TokenStream> = Vec::with_capacity(input.sig.inputs.len() * 2);
    let mut decodings : Vec<proc_macro2::TokenStream> = Vec::with_capacity(input.sig.inputs.len());

    for i in (&input.sig.inputs).into_iter() {
        match i {
            syn::FnArg::Receiver(_) => {
                return token_stream_with_error(original, syn::Error::new(i.span(), format!("handler function does not support 'self' receiver")));
            },
            syn::FnArg::Typed(pat_type) => {
                match &*pat_type.pat {
                    syn::Pat::Ident(v) => {
                        let var_name = v.ident.clone();
                        if final_config.module_type == ModuleType::Store && var_name.to_string().ends_with("_idx") {
                            args.push(quote! { #pat_type });
                            continue
                        }
                        let var_ptr = format_ident!("{}_ptr",var_name);
                        let var_len = format_ident!("{}_len",var_name);
                        args.push(quote! { #var_ptr: *mut u8 });
                        args.push(quote! { #var_len: usize });

                        let argument_type = &*pat_type.ty;
                        decodings.push(quote! { let #var_name: #argument_type = substreams::proto::decode_ptr(#var_ptr, #var_len).unwrap(); })
                    },
                    _ => {
                        return token_stream_with_error(original, syn::Error::new(pat_type.span(), format!("unknown argument type")));
                    }
                }
            },
        }
    }


    match final_config.module_type {
        ModuleType::Store => build_store_handler(input, args, decodings),
        ModuleType::Map => build_map_handler(input, args, decodings)
    }
}

fn parse_func_output(final_config: &FinalConfiguration, output: syn::ReturnType) -> Result<(), syn::Error> {
    match final_config.module_type {
        ModuleType::Map => {
            if output == syn::ReturnType::Default {
                return Err(syn::Error::new(Span::call_site(), "Module of type Map should have a return of type Result<YOUR_TYPE, SubstreamError>"));
            }

            let expected = vec!["-".to_owned(), ">".to_owned(), "Result".to_owned()];
            let mut index = 0;
            let mut valid = true;
            for i in output.into_token_stream().into_iter() {
                if index == expected.len() {
                    if valid {
                        return Ok(())
                    } else {
                        return Err(syn::Error::new(Span::call_site(), "Module of type Map should return a Result<>"));
                    }
                }
                if i.to_string() != expected[index] {
                    valid = false
                }
                index += 1;
            }
            return Ok(())
        },
        ModuleType::Store => {
            if output != syn::ReturnType::Default {
                return Err(syn::Error::new(Span::call_site(), "Module of type Store should not have a return statement"));
            }
            return Ok(())
        }
    }
}

fn build_map_handler(input: syn::ItemFn, collected_args: Vec<proc_macro2::TokenStream>, decodings: Vec<proc_macro2::TokenStream>) -> TokenStream {
    let body = &input.block;
    let header = quote! {
        #[no_mangle]
    };
    let func_name = input.sig.ident.clone();
    let lambda_return = input.sig.output.clone();
    let lambda = quote! {
        let func = || #lambda_return {
            #(#decodings)*
            #body
        };
    };
    let result = quote! {
        #header
        pub extern "C" fn #func_name(#(#collected_args),*){
            substreams::register_panic_hook();
            #lambda
            let result = func();
            if result.is_err() {
                panic!(result.err().unwrap())
            }
            substreams::output(result.unwrap());
        }
    };
    result.into()
}

fn build_store_handler(input: syn::ItemFn, collected_args: Vec<proc_macro2::TokenStream>, decodings: Vec<proc_macro2::TokenStream>) -> TokenStream {
    let body = &input.block;
    let header = quote! {
        #[no_mangle]
    };
    let func_name = input.sig.ident.clone();
    let result = quote! {
        #header
        pub extern "C" fn #func_name(#(#collected_args),*){
            substreams::register_panic_hook();
            #(#decodings)*
            #body
        }
    };
    result.into()
}


fn token_stream_with_error(mut tokens: TokenStream, error: syn::Error) -> TokenStream {
    tokens.extend(TokenStream::from(error.into_compile_error()));
    tokens
}