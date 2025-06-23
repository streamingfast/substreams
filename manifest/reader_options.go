package manifest

import "github.com/jhump/protoreflect/desc"

type Option func(r *Reader) *Reader

func SkipSourceCodeReader() Option {
	return func(r *Reader) *Reader {
		r.validation.SkipSourceCodeImportValidation = true
		return r
	}
}

func SkipModuleOutputTypeValidationReader() Option {
	return func(r *Reader) *Reader {
		r.validation.SkipModuleOutputTypeValidation = true
		return r
	}
}

func SkipPackageValidationReader() Option {
	return func(r *Reader) *Reader {
		r.validation.SkipPackageValidation = true
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

func WithRegistryURL(url string) Option {
	return func(r *Reader) *Reader {
		r.registryURL = url
		return r
	}
}

func WithProtoPath(protoPath string) Option {
	return func(r *Reader) *Reader {
		r.protoPath = protoPath
		return r
	}
}

func WithProtoDescriptorSet(protoDescriptorSet string) Option {
	return func(r *Reader) *Reader {
		r.protoDescriptorSet = protoDescriptorSet
		return r
	}
}
