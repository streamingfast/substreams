package templates

const ModRsTemplate = `{{$modName := . -}}
pub mod {{$modName }};
`
