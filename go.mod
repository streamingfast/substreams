module github.com/streamingfast/substreams

go 1.25.0

toolchain go1.25.4

require (
	github.com/golang/protobuf v1.5.4
	github.com/jhump/protoreflect v1.14.0
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.5
	github.com/streamingfast/bstream v0.0.2-0.20260211203220-e95f72b706fa
	github.com/streamingfast/cli v0.0.4-0.20250815192146-d8a233ec3d0b
	github.com/streamingfast/dauth v0.0.0-20251218134044-fb716c7172b4
	github.com/streamingfast/dbin v0.9.1-0.20231117225723-59790c798e2c
	github.com/streamingfast/derr v0.0.0-20250814163534-bd7407bd89d7
	github.com/streamingfast/dgrpc v0.0.0-20260213152215-f3029d261635
	github.com/streamingfast/dhttp v0.1.3-0.20251218140957-6d46b8f12eb1
	github.com/streamingfast/dstore v0.1.3-0.20260113210117-94d66eda2027
	github.com/streamingfast/logging v0.0.0-20260108192805-38f96de0a641
	github.com/streamingfast/pbgo v0.0.6-0.20250120164644-a58d8066ab4b
	github.com/stretchr/testify v1.11.1
	github.com/yourbasic/graph v0.0.0-20210606180040-8ecfec1c2869
	go.uber.org/zap v1.27.1
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

require (
	buf.build/gen/go/bufbuild/reflect/connectrpc/go v1.16.1-20240117202343-bf8f65e8876c.1
	buf.build/gen/go/bufbuild/reflect/protocolbuffers/go v1.33.0-20240117202343-bf8f65e8876c.1
	connectrpc.com/connect v1.19.1
	github.com/KimMachineGun/automemlimit v0.7.5
	github.com/RoaringBitmap/roaring v1.9.1
	github.com/abourget/llerrgroup v0.2.0
	github.com/alecthomas/chroma v0.10.0
	github.com/alecthomas/participle v0.7.1
	github.com/bmatcuk/doublestar/v4 v4.6.1
	github.com/bobg/go-generics/v2 v2.2.2
	github.com/bytecodealliance/wasmtime-go/v41 v41.0.0
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/charmbracelet/bubbles v0.20.0
	github.com/charmbracelet/bubbletea v1.1.0
	github.com/charmbracelet/glamour v0.7.0
	github.com/charmbracelet/huh v0.6.0
	github.com/charmbracelet/huh/spinner v0.0.0-20240806005253-b7436a76999a
	github.com/charmbracelet/lipgloss v1.0.0
	github.com/charmbracelet/x/ansi v0.4.2
	github.com/dustin/go-humanize v1.0.1
	github.com/golang-cz/textcase v1.2.1
	github.com/google/uuid v1.6.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/itchyny/gojq v0.12.12
	github.com/klauspost/connect-compress/v2 v2.1.1
	github.com/lithammer/dedent v1.1.0
	github.com/mattn/go-isatty v0.0.20
	github.com/mitchellh/go-testing-interface v1.14.1
	github.com/mostynb/go-grpc-compression v1.2.3
	github.com/muesli/reflow v0.3.0
	github.com/muesli/termenv v0.15.3-0.20240618155329-98d742f6907a
	github.com/orcaman/concurrent-map/v2 v2.0.1
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
	github.com/prometheus/client_golang v1.16.0
	github.com/protocolbuffers/protoscope v0.0.0-20221109213918-8e7a6aafa2c9
	github.com/schollz/closestmatch v2.1.0+incompatible
	github.com/shopspring/decimal v1.3.1
	github.com/streamingfast/dmetering v0.0.0-20251027175535-4fd530934b97
	github.com/streamingfast/dmetrics v0.0.0-20260109212625-35256f512c62
	github.com/streamingfast/dsession v0.0.0-20251029144057-b94d1030e142
	github.com/streamingfast/dummy-blockchain v1.7.3
	github.com/streamingfast/firehose-ethereum/types v0.0.0-20251113151010-c9c94d64348a
	github.com/streamingfast/firehose-networks v0.2.2
	github.com/streamingfast/sf-tracing v0.0.0-20251218140752-bafd5572499f
	github.com/streamingfast/shutter v1.5.0
	github.com/streamingfast/substreams-sdk-go v0.0.0-20240110154316-5fb21a7a330b
	github.com/streamingfast/substreams-sink-files/v2 v2.3.1
	github.com/test-go/testify v1.1.4
	github.com/tetratelabs/wazero v1.8.0
	github.com/tidwall/pretty v1.2.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.64.0
	go.opentelemetry.io/otel v1.39.0
	go.opentelemetry.io/otel/trace v1.39.0
	go.uber.org/atomic v1.11.0
	golang.org/x/mod v0.29.0
	golang.org/x/net v0.47.0
	golang.org/x/oauth2 v0.33.0
	google.golang.org/grpc v1.77.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	cel.dev/expr v0.24.0 // indirect
	cloud.google.com/go/auth v0.17.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	connectrpc.com/grpchealth v1.3.0 // indirect
	connectrpc.com/grpcreflect v1.3.0 // indirect
	connectrpc.com/otelconnect v0.8.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.20.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.3 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.6.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.54.0 // indirect
	github.com/alecthomas/chroma/v2 v2.8.0 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/aws/aws-sdk-go-v2 v1.32.6 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.7 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.28.6 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.47 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.21 // indirect
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.17.43 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.4.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.71.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.24.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.28.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.2 // indirect
	github.com/aws/smithy-go v1.22.1 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.12.0 // indirect
	github.com/bobg/go-generics/v3 v3.5.0 // indirect
	github.com/catppuccin/go v0.2.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/charmbracelet/x/exp/strings v0.0.0-20240722160745-212f7b056ed0 // indirect
	github.com/charmbracelet/x/term v0.2.0 // indirect
	github.com/envoyproxy/go-control-plane v0.14.0 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.36.0 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/gorilla/schema v1.0.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/holiman/uint256 v1.3.1 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/manifoldco/promptui v0.9.0 // indirect
	github.com/minio/minlz v1.0.1 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/parquet-go/parquet-go v0.23.0 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pinax-network/graph-networks-libs/packages/golang v0.7.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/rs/cors v1.10.0 // indirect
	github.com/sahilm/fuzzy v0.1.1 // indirect
	github.com/segmentio/encoding v0.4.0 // indirect
	github.com/sercand/kuberesolver/v5 v5.1.1 // indirect
	github.com/spiffe/go-spiffe/v2 v2.6.0 // indirect
	github.com/streamingfast/dhammer v0.0.0-20220506192416-3797a7906da2 // indirect
	github.com/streamingfast/validator v0.0.0-20231124184318-71ec8080e4ae // indirect
	github.com/thedevsaddam/govalidator v1.9.6 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.63.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.39.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	golang.org/x/exp v0.0.0-20250813145105-42675adae3e6 // indirect
	golang.org/x/time v0.14.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)

require (
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/iam v1.5.3 // indirect
	cloud.google.com/go/monitoring v1.24.2 // indirect
	cloud.google.com/go/storage v1.59.0 // indirect
	cloud.google.com/go/trace v1.11.6 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.30.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace v1.30.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.54.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator v0.54.0 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blendle/zapdriver v1.3.2-0.20200203083823-9200777f8a3d // indirect
	github.com/bufbuild/protocompile v0.4.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chzyer/readline v1.5.0 // indirect
	github.com/cncf/xds/go v0.0.0-20251022180443-0feb69152e9f // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.7 // indirect
	github.com/googleapis/gax-go/v2 v2.15.0 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/itchyny/timefmt-go v0.1.5 // indirect
	github.com/klauspost/compress v1.18.3
	github.com/logrusorgru/aurora v2.0.3+incompatible // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.16
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/microcosm-cc/bluemonday v1.0.25 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mr-tron/base58 v1.2.0
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/olekukonko/tablewriter v0.0.5
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/paulbellamy/ratecounter v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sethvargo/go-retry v0.2.3 // indirect
	github.com/spf13/afero v1.10.0 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.15.0 // indirect
	github.com/streamingfast/opaque v0.0.0-20210811180740-0c01d37ea308 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	github.com/teris-io/shortid v0.0.0-20171029131806-771a37caa5cf // indirect
	github.com/tidwall/gjson v1.18.0
	github.com/yuin/goldmark v1.5.4 // indirect
	github.com/yuin/goldmark-emoji v1.0.2 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/zipkin v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk v1.39.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.44.0 // indirect
	golang.org/x/sync v0.18.0
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/term v0.37.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/api v0.256.0 // indirect
	google.golang.org/genproto v0.0.0-20250922171735-9219d122eba9 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	rogchap.com/v8go v0.9.0
)

retract v1.0.2 // Published at wrong tag.

replace (
	github.com/jhump/protoreflect => github.com/streamingfast/protoreflect v0.0.0-20231205191344-4b629d20ce8d
	github.com/yourbasic/graph v0.0.0-20210606180040-8ecfec1c2869 => github.com/streamingfast/graph v0.0.0-20220329181048-a5710712d873
	rogchap.com/v8go => github.com/streamingfast/v8go v0.0.0-20250609192103-f9b5beca42f2
)
