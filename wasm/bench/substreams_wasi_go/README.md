```
GOOS=wasip1 GOARCH=wasm go build -o main.wasm main.go
```

```
protoc -I./proto \
--go_out=. --plugin protoc-gen-go="/Users/cbillett/go/bin/protoc-gen-go" \
--go-vtproto_out=. --plugin protoc-gen-go-vtproto="/Users/cbillett/go/bin/protoc-gen-go-vtproto" \
--go-vtproto_opt=features=marshal+unmarshal+size \
input.proto;
```


