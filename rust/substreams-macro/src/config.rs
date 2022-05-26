// use proc_macro2::{Span};
// type AttributeArgs = syn::punctuated::Punctuated<syn::NestedMeta, syn::Token![,]>;

#[derive(Clone, Copy, PartialEq)]
pub enum ModuleType {
    Store,
    Map,
}

impl ModuleType {
    // fn from_str(s: &str) -> Result<ModuleType, String> {
    //     match s {
    //         "store" => Ok(ModuleType::Store),
    //         "map" => Ok(ModuleType::Map),
    //         _ => Err(format!("No such module type `{}`. The modules types are `store` and `map`.", s)),
    //     }
    // }
}

pub struct FinalConfiguration {
    pub module_type: ModuleType
}

// struct Configuration {
//     module_type: Option<ModuleType>
// }
//
// impl Configuration {
//     fn new() -> Self {
//         Configuration {
//             module_type: None,
//         }
//     }
//
//     fn set_module_type(&mut self, runtime: syn::Lit, span: Span) -> Result<(), syn::Error> {
//         if self.module_type.is_some() {
//             return Err(syn::Error::new(span, "`type` set multiple times."));
//         }
//
//         let runtime_str = parse_string(runtime, span, "type")?;
//         let mod_type =
//             ModuleType::from_str(&runtime_str).map_err(|err| syn::Error::new(span, err))?;
//         self.module_type = Some(mod_type);
//         Ok(())
//     }
//
//     fn build(&self) -> Result<FinalConfiguration, syn::Error> {
//         let mod_type = self.module_type.unwrap_or(ModuleType::Map);
//         Ok(FinalConfiguration {
//             module_type: mod_type,
//         })
//     }
// }

// fn parse_string(int: syn::Lit, span: Span, field: &str) -> Result<String, syn::Error> {
//     match int {
//         syn::Lit::Str(s) => Ok(s.value()),
//         syn::Lit::Verbatim(s) => Ok(s.to_string()),
//         _ => Err(syn::Error::new(
//             span,
//             format!("Failed to parse value of `{}` as string.", field),
//         )),
//     }
// }


// fn build_config(
//     args: AttributeArgs,
// ) -> Result<FinalConfiguration, syn::Error> {
//
//     let mut config = Configuration::new();
//
//     for arg in args {
//         match arg {
//             syn::NestedMeta::Meta(syn::Meta::NameValue(namevalue)) => {
//                 let ident = namevalue
//                     .path
//                     .get_ident()
//                     .ok_or_else(|| {
//                         syn::Error::new_spanned(&namevalue, "Must have specified ident")
//                     })?
//                     .to_string()
//                     .to_lowercase();
//                 match ident.as_str() {
//                     "type" => {
//                         config.set_module_type(
//                             namevalue.lit.clone(),
//                             syn::spanned::Spanned::span(&namevalue.lit),
//                         )?;
//                     }
//                     name => {
//                         let msg = format!(
//                             "Unknown attribute {} is specified; expected one of: `type`",
//                             name,
//                         );
//                         return Err(syn::Error::new_spanned(namevalue, msg));
//                     }
//                 }
//             }
//             other => {
//                 return Err(syn::Error::new_spanned(
//                     other,
//                     "Unknown attribute inside the macro",
//                 ));
//             }
//         }
//     }
//     config.build()
// }