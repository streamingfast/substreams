# Integration tests

To run the integration test you need to set your ENV var `SUBSTREAMS_INTEGRATION_TESTS=true`. By default
the tracing is disabled in integration tests. To enable the tracing following the steps below


## Tracing

Note: I'm getting an error with jaeger

1) Run a tracing client (jaeger, zipkin...). Don't run both, choose one


```shell
# running zipkin
docker run -p 9411:9411 openzipkin/zipkin
# running zipkin jaeger
docker run --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4317:4317 \
  -p 4318:4318 \
  jaegertracing/all-in-one:1.35

```

2) Run test with ENV var 
 * Zipkin: `SF_TRACING=zipkin://localhost:9411?scheme=http`
 * Jaeger: `SF_TRACING=jaeger://localhost:4317?scheme=http`

3) Wait a few seconds after the test is complete to see the trace in your browser:
   * Zipkin: http://localhost:9411/
   * Jaeger: http://localhost:16686/


