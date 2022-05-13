module github.com/streamingfast/substreams

go 1.17

require (
	github.com/abourget/llerrgroup v0.2.0
	github.com/drone/envsubst v1.0.2
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/jhump/protoreflect v1.12.0
	github.com/spf13/cobra v1.3.0
	github.com/spf13/pflag v1.0.5
	github.com/streamingfast/bstream v0.0.2-0.20220505155906-a0834b9c5258
	github.com/streamingfast/dstore v0.1.1-0.20220419183635-aad7bcb15b8e
	github.com/streamingfast/firehose v0.1.1-0.20220427051727-9b108461d3a6
	github.com/streamingfast/logging v0.0.0-20220511154537-ce373d264338
	github.com/streamingfast/pbgo v0.0.6-0.20220428192744-f80aee7d4688
	github.com/stretchr/testify v1.7.1
	github.com/test-go/testify v1.1.4
	github.com/yourbasic/graph v0.0.0-20210606180040-8ecfec1c2869
	go.uber.org/zap v1.21.0
	google.golang.org/protobuf v1.27.1
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

replace (
	github.com/wasmerio/wasmer-go v1.0.4 => github.com/streamingfast/wasmer-go v1.0.5-0.20220512181019-cd8b01df1543
	github.com/yourbasic/graph v0.0.0-20210606180040-8ecfec1c2869 => github.com/streamingfast/graph v0.0.0-20220329181048-a5710712d873
)

require (
	github.com/bmatcuk/doublestar/v4 v4.0.2
	github.com/dustin/go-humanize v1.0.0
	github.com/iancoleman/strcase v0.2.0
	github.com/jmoiron/sqlx v1.3.4
	github.com/jszwec/csvutil v1.6.0
	github.com/lib/pq v1.10.5
	github.com/streamingfast/cli v0.0.3
	github.com/streamingfast/dgrpc v0.0.0-20220301153539-536adf71b594
	github.com/wasmerio/wasmer-go v1.0.4
	golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8
	google.golang.org/grpc v1.44.0
)

require (
	cloud.google.com/go v0.100.2 // indirect
	cloud.google.com/go/compute v1.3.0 // indirect
	cloud.google.com/go/iam v0.3.0 // indirect
	cloud.google.com/go/monitoring v1.4.0 // indirect
	cloud.google.com/go/storage v1.21.0 // indirect
	cloud.google.com/go/trace v1.2.0 // indirect
	contrib.go.opencensus.io/exporter/stackdriver v0.13.8 // indirect
	contrib.go.opencensus.io/exporter/zipkin v0.1.1 // indirect
	github.com/Azure/azure-pipeline-go v0.2.3 // indirect
	github.com/Azure/azure-storage-blob-go v0.14.0 // indirect
	github.com/RoaringBitmap/roaring v0.9.4 // indirect
	github.com/aws/aws-sdk-go v1.37.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.2.0 // indirect
	github.com/blendle/zapdriver v1.3.1 // indirect
	github.com/census-instrumentation/opencensus-proto v0.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.5.7 // indirect
	github.com/google/uuid v1.2.0 // indirect
	github.com/googleapis/gax-go/v2 v2.1.1 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/juju/ansiterm v0.0.0-20180109212912-720a0952cc2a // indirect
	github.com/klauspost/compress v1.10.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lithammer/dedent v1.1.0 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/lunixbochs/vtclean v0.0.0-20180621232353-2d01aacdc34a // indirect
	github.com/manifoldco/promptui v0.8.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-ieproxy v0.0.1 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mitchellh/go-testing-interface v1.14.1 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/openzipkin/zipkin-go v0.1.6 // indirect
	github.com/pelletier/go-toml/v2 v2.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.12.1 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/streamingfast/atm v0.0.0-20220131151839-18c87005e680 // indirect
	github.com/streamingfast/dauth v0.0.0-20220404140613-a40f4cd81626 // indirect
	github.com/streamingfast/dbin v0.0.0-20210809205249-73d5eca35dc5 // indirect
	github.com/streamingfast/dmetering v0.0.0-20220307162406-37261b4b3de9 // indirect
	github.com/streamingfast/dmetrics v0.0.0-20210811180524-8494aeb34447 // indirect
	github.com/streamingfast/dtracing v0.0.0-20210811175635-d55665d3622a // indirect
	github.com/streamingfast/opaque v0.0.0-20210811180740-0c01d37ea308 // indirect
	github.com/streamingfast/shutter v1.5.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292 // indirect
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20220209214540-3681064d5158 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/api v0.70.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220222213610-43724f9ea8cf // indirect
)
