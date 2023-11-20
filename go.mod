module github.com/streamingfast/substreams

go 1.21

require (
	github.com/abourget/llerrgroup v0.2.0
	github.com/golang/protobuf v1.5.3
	github.com/jhump/protoreflect v1.14.0
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/streamingfast/bstream v0.0.2-0.20230829131224-b9272048dc6a
	github.com/streamingfast/cli v0.0.4-0.20230825151644-8cc84512cd80
	github.com/streamingfast/derr v0.0.0-20230515163924-8570aaa43fe1
	github.com/streamingfast/dgrpc v0.0.0-20230929132851-893fc52687fa
	github.com/streamingfast/dstore v0.1.1-0.20230620124109-3924b3b36c77
	github.com/streamingfast/logging v0.0.0-20230608130331-f22c91403091
	github.com/streamingfast/pbgo v0.0.6-0.20221020131607-255008258d28
	github.com/stretchr/testify v1.8.4
	github.com/yourbasic/graph v0.0.0-20210606180040-8ecfec1c2869
	go.uber.org/zap v1.26.0
	google.golang.org/protobuf v1.31.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/yourbasic/graph v0.0.0-20210606180040-8ecfec1c2869 => github.com/streamingfast/graph v0.0.0-20220329181048-a5710712d873

replace github.com/bytecodealliance/wasmtime-go/v4 => github.com/streamingfast/wasmtime-go/v4 v4.0.0-freemem3

require (
	github.com/alecthomas/chroma v0.10.0
	github.com/bufbuild/connect-go v1.10.0
	github.com/bufbuild/connect-grpcreflect-go v1.0.0
	github.com/bytecodealliance/wasmtime-go/v4 v4.0.0
	github.com/charmbracelet/bubbles v0.15.0
	github.com/charmbracelet/bubbletea v0.23.1
	github.com/charmbracelet/glamour v0.6.0
	github.com/charmbracelet/lipgloss v0.6.0
	github.com/docker/cli v24.0.6+incompatible
	github.com/dustin/go-humanize v1.0.1
	github.com/gertd/go-pluralize v0.2.1
	github.com/google/uuid v1.3.0
	github.com/iancoleman/strcase v0.2.0
	github.com/ipfs/go-ipfs-api v0.6.0
	github.com/itchyny/gojq v0.12.12
	github.com/lithammer/dedent v1.1.0
	github.com/manifoldco/promptui v0.9.0
	github.com/mattn/go-isatty v0.0.17
	github.com/mitchellh/go-testing-interface v1.14.1
	github.com/muesli/reflow v0.3.0
	github.com/muesli/termenv v0.13.0
	github.com/prometheus/client_golang v1.16.0
	github.com/rs/cors v1.10.0
	github.com/schollz/closestmatch v2.1.0+incompatible
	github.com/shopspring/decimal v1.3.1
	github.com/streamingfast/dauth v0.0.0-20231120142446-843f4e045cc2
	github.com/streamingfast/dbin v0.0.0-20210809205249-73d5eca35dc5
	github.com/streamingfast/dhttp v0.0.2-0.20220314180036-95936809c4b8
	github.com/streamingfast/dmetering v0.0.0-20231120142327-a3405f0eed83
	github.com/streamingfast/dmetrics v0.0.0-20230919161904-206fa8ebd545
	github.com/streamingfast/eth-go v0.0.0-20230410173454-433bd8803da1
	github.com/streamingfast/sf-tracing v0.0.0-20230616174903-cd2ade641ca9
	github.com/streamingfast/shutter v1.5.0
	github.com/streamingfast/substreams-sink-sql v1.0.1-0.20231107184906-39740d2091cd
	github.com/tetratelabs/wazero v1.1.0
	github.com/tidwall/pretty v1.2.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.44.0
	go.opentelemetry.io/otel v1.18.0
	go.opentelemetry.io/otel/trace v1.18.0
	go.uber.org/atomic v1.10.0
	golang.org/x/mod v0.12.0
	golang.org/x/net v0.15.0
	golang.org/x/oauth2 v0.10.0
	google.golang.org/grpc v1.58.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/bobg/go-generics/v2 v2.1.1 // indirect
	github.com/bufbuild/connect-grpchealth-go v1.1.1 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/google/s2a-go v0.1.4 // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230711160842-782d3b101e98 // indirect
)

require (
	cloud.google.com/go v0.110.4 // indirect
	cloud.google.com/go/compute v1.21.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v1.1.1 // indirect
	cloud.google.com/go/monitoring v1.15.1 // indirect
	cloud.google.com/go/storage v1.30.1 // indirect
	cloud.google.com/go/trace v1.10.1 // indirect
	contrib.go.opencensus.io/exporter/stackdriver v0.13.10 // indirect
	contrib.go.opencensus.io/exporter/zipkin v0.1.1 // indirect
	github.com/Azure/azure-pipeline-go v0.2.3 // indirect
	github.com/Azure/azure-storage-blob-go v0.14.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v0.32.3 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace v1.15.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.39.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator v0.0.0-20221018185641-36f91511cfd7 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aws/aws-sdk-go v1.44.325 // indirect
	github.com/aymanbagabas/go-osc52 v1.2.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blendle/zapdriver v1.3.2-0.20200203083823-9200777f8a3d // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.2.0 // indirect
	github.com/bufbuild/connect-opentelemetry-go v0.3.0 // indirect
	github.com/bufbuild/protocompile v0.4.0 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/chzyer/readline v1.5.0 // indirect
	github.com/cncf/udpa/go v0.0.0-20220112060539-c52dc94e7fbe // indirect
	github.com/cncf/xds/go v0.0.0-20230607035331-e9ce68804cb4 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/crackcomm/go-gitignore v0.0.0-20170627025303-887ab5e44cc3 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/envoyproxy/go-control-plane v0.11.1 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.0.2 // indirect
	github.com/eoscanada/eos-go v0.9.1-0.20200415144303-2adb25bcdeca // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.3 // indirect
	github.com/googleapis/gax-go/v2 v2.11.0 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/schema v1.0.2 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.16.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/holiman/uint256 v1.2.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ipfs/boxo v0.8.0 // indirect
	github.com/ipfs/go-cid v0.4.0 // indirect
	github.com/itchyny/timefmt-go v0.1.5 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.16.5 // indirect
	github.com/klauspost/cpuid/v2 v2.2.3 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-flow-metrics v0.1.0 // indirect
	github.com/libp2p/go-libp2p v0.26.3 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-ieproxy v0.0.1 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/microcosm-cc/bluemonday v1.0.21 // indirect
	github.com/minio/sha256-simd v1.0.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/muesli/ansi v0.0.0-20211031195517-c9f0611b6c70 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.8.0 // indirect
	github.com/multiformats/go-multibase v0.1.1 // indirect
	github.com/multiformats/go-multicodec v0.8.1 // indirect
	github.com/multiformats/go-multihash v0.2.1 // indirect
	github.com/multiformats/go-multistream v0.4.1 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/openzipkin/zipkin-go v0.4.1 // indirect
	github.com/paulbellamy/ratecounter v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.0 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/sethvargo/go-retry v0.2.3 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/afero v1.9.3 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.15.0 // indirect
	github.com/streamingfast/atm v0.0.0-20220131151839-18c87005e680 // indirect
	github.com/streamingfast/dtracing v0.0.0-20220305214756-b5c0e8699839 // indirect
	github.com/streamingfast/opaque v0.0.0-20210811180740-0c01d37ea308 // indirect
	github.com/streamingfast/validator v0.0.0-20210812013448-b9da5752ce14 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	github.com/teris-io/shortid v0.0.0-20171029131806-771a37caa5cf // indirect
	github.com/thedevsaddam/govalidator v1.9.6 // indirect
	github.com/tidwall/gjson v1.14.1 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/sjson v1.0.4 // indirect
	github.com/whyrusleeping/tar-utils v0.0.0-20180509141711-8c6c8ba81d5c // indirect
	github.com/yuin/goldmark v1.5.2 // indirect
	github.com/yuin/goldmark-emoji v1.0.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.9.0 // indirect
	go.opentelemetry.io/otel/exporters/jaeger v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.18.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.18.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.15.1 // indirect
	go.opentelemetry.io/otel/exporters/zipkin v1.15.1 // indirect
	go.opentelemetry.io/otel/metric v1.18.0 // indirect
	go.opentelemetry.io/otel/sdk v1.18.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/crypto v0.13.0 // indirect
	golang.org/x/exp v0.0.0-20230713183714-613f0c0eb8a1
	golang.org/x/sync v0.3.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/term v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/api v0.126.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230711160842-782d3b101e98 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	lukechampine.com/blake3 v1.1.7 // indirect
)

retract v1.0.2 // Published at wrong tag.

replace github.com/jhump/protoreflect => github.com/streamingfast/protoreflect v0.0.0-20230414203421-018294174fdc
