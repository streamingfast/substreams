module github.com/streamingfast/substreams

go 1.19

require (
	github.com/abourget/llerrgroup v0.2.0
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/jhump/protoreflect v1.12.0
	github.com/spf13/cobra v1.6.1
	github.com/spf13/pflag v1.0.5
	github.com/streamingfast/bstream v0.0.2-0.20230510131449-6b591d74130d
	github.com/streamingfast/cli v0.0.4-0.20220630165922-bc58c6666fc8
	github.com/streamingfast/derr v0.0.0-20230515163924-8570aaa43fe1
	github.com/streamingfast/dgrpc v0.0.0-20230417152409-2ee737f143dd
	github.com/streamingfast/dstore v0.1.1-0.20230511202333-4f4ccf11a05f
	github.com/streamingfast/logging v0.0.0-20220511154537-ce373d264338
	github.com/streamingfast/pbgo v0.0.6-0.20220630154121-2e8bba36234e // indirect
	github.com/stretchr/testify v1.8.2
	github.com/yourbasic/graph v0.0.0-20210606180040-8ecfec1c2869
	go.uber.org/zap v1.21.0
	google.golang.org/protobuf v1.30.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/yourbasic/graph v0.0.0-20210606180040-8ecfec1c2869 => github.com/streamingfast/graph v0.0.0-20220329181048-a5710712d873

replace github.com/bytecodealliance/wasmtime-go/v4 => github.com/streamingfast/wasmtime-go/v4 v4.0.0-freemem3

require (
	github.com/alecthomas/chroma v0.10.0
	github.com/bufbuild/connect-go v1.1.0
	github.com/bufbuild/connect-grpcreflect-go v1.0.0
	github.com/bytecodealliance/wasmtime-go/v4 v4.0.0
	github.com/charmbracelet/bubbles v0.15.0
	github.com/charmbracelet/bubbletea v0.23.1
	github.com/charmbracelet/glamour v0.6.0
	github.com/charmbracelet/lipgloss v0.6.0
	github.com/dustin/go-humanize v1.0.0
	github.com/gertd/go-pluralize v0.2.1
	github.com/google/uuid v1.3.0
	github.com/iancoleman/strcase v0.2.0
	github.com/itchyny/gojq v0.12.12
	github.com/manifoldco/promptui v0.9.0
	github.com/mattn/go-isatty v0.0.17
	github.com/mitchellh/go-testing-interface v1.14.1
	github.com/muesli/reflow v0.3.0
	github.com/muesli/termenv v0.13.0
	github.com/prometheus/client_golang v1.12.1
	github.com/rs/cors v1.8.3
	github.com/schollz/closestmatch v2.1.0+incompatible
	github.com/streamingfast/dauth v0.0.0-20221027185237-b209f25fa3ff
	github.com/streamingfast/dbin v0.0.0-20210809205249-73d5eca35dc5
	github.com/streamingfast/dhttp v0.0.2-0.20220314180036-95936809c4b8
	github.com/streamingfast/dmetrics v0.0.0-20221107142404-e88fe183f07d
	github.com/streamingfast/eth-go v0.0.0-20230410173454-433bd8803da1
	github.com/streamingfast/sf-tracing v0.0.0-20230519113358-f3dc5e582d12
	github.com/streamingfast/shutter v1.5.0
	github.com/tidwall/pretty v1.2.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.36.4
	go.opentelemetry.io/otel v1.15.1
	go.opentelemetry.io/otel/trace v1.15.1
	go.uber.org/atomic v1.10.0
	golang.org/x/mod v0.8.0
	golang.org/x/net v0.8.0
	golang.org/x/oauth2 v0.6.0
	google.golang.org/grpc v1.54.0
)

require (
	cloud.google.com/go v0.110.0 // indirect
	cloud.google.com/go/compute v1.18.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v0.12.0 // indirect
	cloud.google.com/go/monitoring v1.12.0 // indirect
	cloud.google.com/go/storage v1.30.1 // indirect
	cloud.google.com/go/trace v1.8.0 // indirect
	contrib.go.opencensus.io/exporter/stackdriver v0.13.10 // indirect
	contrib.go.opencensus.io/exporter/zipkin v0.1.1 // indirect
	github.com/Azure/azure-pipeline-go v0.2.3 // indirect
	github.com/Azure/azure-storage-blob-go v0.14.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v0.32.3 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace v1.8.6 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.32.6 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go v1.44.233 // indirect
	github.com/aymanbagabas/go-osc52 v1.2.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blendle/zapdriver v1.3.1 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.2.0 // indirect
	github.com/bufbuild/protocompile v0.4.0 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e // indirect
	github.com/cncf/udpa/go v0.0.0-20220112060539-c52dc94e7fbe // indirect
	github.com/cncf/xds/go v0.0.0-20230105202645-06c439db220b // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/dlclark/regexp2 v1.4.0 // indirect
	github.com/envoyproxy/go-control-plane v0.10.3 // indirect
	github.com/envoyproxy/protoc-gen-validate v0.9.1 // indirect
	github.com/eoscanada/eos-go v0.9.1-0.20200415144303-2adb25bcdeca // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.3 // indirect
	github.com/googleapis/gax-go/v2 v2.7.1 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/handlers v0.0.0-20181012153334-350d97a79266 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/schema v1.0.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.11.3 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/holiman/uint256 v1.2.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/itchyny/timefmt-go v0.1.5 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.15.11 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/lithammer/dedent v1.1.0 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mattn/go-ieproxy v0.0.1 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/microcosm-cc/bluemonday v1.0.21 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/muesli/ansi v0.0.0-20211031195517-c9f0611b6c70 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/openzipkin/zipkin-go v0.4.1 // indirect
	github.com/paulbellamy/ratecounter v0.2.0 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/sethvargo/go-retry v0.2.3 // indirect
	github.com/spf13/afero v1.9.2 // indirect
	github.com/spf13/cast v1.3.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/viper v1.7.0 // indirect
	github.com/streamingfast/atm v0.0.0-20220131151839-18c87005e680 // indirect
	github.com/streamingfast/dtracing v0.0.0-20220305214756-b5c0e8699839 // indirect
	github.com/streamingfast/opaque v0.0.0-20210811180740-0c01d37ea308 // indirect
	github.com/streamingfast/validator v0.0.0-20210812013448-b9da5752ce14 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	github.com/teris-io/shortid v0.0.0-20171029131806-771a37caa5cf // indirect
	github.com/thedevsaddam/govalidator v1.9.6 // indirect
	github.com/tidwall/gjson v1.14.1 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/sjson v1.0.4 // indirect
	github.com/yuin/goldmark v1.5.2 // indirect
	github.com/yuin/goldmark-emoji v1.0.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.9.0 // indirect
	go.opentelemetry.io/otel/exporters/jaeger v1.15.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.15.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.15.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.15.1 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.15.1 // indirect
	go.opentelemetry.io/otel/exporters/zipkin v1.15.1 // indirect
	go.opentelemetry.io/otel/sdk v1.15.1 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/crypto v0.1.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.7.0 // indirect
	golang.org/x/term v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/api v0.114.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230320184635-7606e756e683 // indirect
	gopkg.in/ini.v1 v1.51.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

retract v1.0.2 // Published at wrong tag.

replace github.com/jhump/protoreflect => github.com/streamingfast/protoreflect v0.0.0-20230414203421-018294174fdc
