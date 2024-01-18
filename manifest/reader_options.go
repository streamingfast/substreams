package manifest

import "github.com/jhump/protoreflect/desc"

type Option func(r *Reader) *Reader

func SkipSourceCodeReader() Option {
	return func(r *Reader) *Reader {
		r.skipSourceCodeImportValidation = true
		return r
	}
}

func SkipModuleOutputTypeValidationReader() Option {
	return func(r *Reader) *Reader {
		r.skipModuleOutputTypeValidation = true
		return r
	}
}

func SkipPackageValidationReader() Option {
	return func(r *Reader) *Reader {
		r.skipPackageValidation = true
		return r
	}
}

func WithOverrideNetwork(network string) Option {
	return func(r *Reader) *Reader {
		r.overrideNetwork = network
		return r
	}
}

func WithOverrideOutputModule(outputModule string) Option {
	return func(r *Reader) *Reader {
		r.overrideOutputModule = outputModule
		return r
	}
}

func WithParams(params map[string]string) Option {
	return func(r *Reader) *Reader {
		r.params = params
		return r
	}
}

func WithCollectProtoDefinitions(f func(protoDefinitions []*desc.FileDescriptor)) Option {
	return func(r *Reader) *Reader {
		r.collectProtoDefinitionsFunc = f
		return r
	}
}
