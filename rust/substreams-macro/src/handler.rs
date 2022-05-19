use proc_macro::{TokenStream};
use std::borrow::BorrowMut;
use std::convert::TryInto;
use proc_macro2::{Ident, Span};
use quote::{quote, quote_spanned, ToTokens, format_ident};
use quote::__private::ext::RepToTokensExt;
use syn::{NestedMeta, parse_quote, PatIdent, PatType, Token};
use syn::parse::Parser;
use syn::ReturnType::Default;
use syn::spanned::Spanned;

// syn::AttributeArgs does not implement syn::Parse
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
                            "Unknown attribute {} is specified; expected one of: `flavor`, `worker_threads`, `start_paused`, `crate`",
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


#[cfg(not(test))] // Work around for rust-lang/rust#62127
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
    let mut input = syn::parse_macro_input!(item as syn::ItemFn);

    let output_result = parse_output(final_config, input.sig.output.clone());
    match output_result {
        Ok(_) => {}
        Err(e) => {
            return token_stream_with_error(original, e)
        }
    }


    let mut new_inputs : syn::punctuated::Punctuated<syn::FnArg, syn::Token![,]> = syn::punctuated::Punctuated::new();


    println!("input sig ident {:?}",input.sig.ident);
    println!("input sig inputs {:?}",input.sig.inputs[0]);
    for i in (&input.sig.inputs).into_iter() {
        let mut  varname_ptr = &format!("ptr");
        let mut  varname_len = &format!("len");
        
        let mut iter = i.into_token_stream().into_iter();
        if let Some(i) = iter.next() {
            varname_ptr = &format!("{}_ptr", i);
            varname_len = &format!("{}_len", i);
        }


        for j in i.into_token_stream().into_iter() {
            println!("input attempt:  {:?}",j);
        }
    }

    // let mut new_inputs = input.sig.inputs.clone();
    // // new_inputs.push();
    // // new_inputs.pop();
    // input.sig.inputs = new_inputs;
    // input.sig.inputs.push(syn::FnArg::parse())
    //
    //
    // println!("input attrs len {:?}",input.attrs.len());
    // println!("input block stmts last {:?}",input.block.stmts.last());
    //
    // for i in (&input.block.stmts).into_iter() {
    //     println!("input stmt {:?}",i);
    // }
    //
    // let config = if input.sig.ident == "main" && !input.sig.inputs.is_empty() {
    //     let msg = "the main function cannot accept arguments";
    //     Err(syn::Error::new_spanned(&input.sig.ident, msg))
    // } else {
    //     AttributeArgs::parse_terminated
    //         .parse(args)
    //         .and_then(|args| build_config(input.clone(), args, false, rt_multi_thread))
    // };
    //
    parse_func(input)
}

fn parse_output(finalConfig: FinalConfiguration,output: syn::ReturnType) -> Result<(), syn::Error> {
    match finalConfig.module_type {
        ModuleType::Map => {
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

fn parse_func(mut input: syn::ItemFn) -> TokenStream {
    let foo = input.block.clone();
    println!("body");
    for i in foo.into_token_stream().into_iter() {
        for j in i.into_token_stream().into_iter() {
            println!("FOO      *  {:?}",j.to_string());
        }
    }
    println!("done");
    // current main function
    let body = &input.block.clone();
    // let brace_token = input.block.brace_token;
    let header = quote! {
        #[no_mangle]
    };


    // let lambdaName = format_ident!("func");
    // let lambdaReturnType : syn::ReturnType =  syn::parse_str("Result<bool,String>").unwrap();

    input.block = syn::parse2(quote! {
        {
            substreams::register_panic_hook();
            #body
        }
    }).expect("Parsing failure");
    // input.block.brace_token = brace_token;



    let func_name = input.sig.ident.clone();
    let transfer_ptr = format_ident!("{}_ptr","transfers");
    let transfer_ptr_arg = quote! {
            #transfer_ptr: *mut u8
    };
    let transfer_len = format_ident!("{}_len","transfers");
    let transfer_len_arg = quote! {
            #transfer_len: usize
    };
    let transfer_decode = quote! {
        let transfers: erc721::Transfers = substreams::proto::decode_ptr(#transfer_ptr, #transfer_len).unwrap();
    };

    let args = vec![transfer_ptr_arg, transfer_len_arg];

    let lambdaReturn = input.sig.output.clone();
    let lambda = quote! {
        let func = || #lambdaReturn {
            #transfer_decode
            #body
        };
    };
    let result = quote! {
        #header

        pub fn #func_name(#(#args),*){
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

fn token_stream_with_error(mut tokens: TokenStream, error: syn::Error) -> TokenStream {
    tokens.extend(TokenStream::from(error.into_compile_error()));
    tokens
}