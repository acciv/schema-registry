FROM rust:latest AS rust-builder
WORKDIR /src
COPY rust/ rust/
RUN cargo build --release --manifest-path rust/Cargo.toml

FROM golang:1.23 AS go-builder
WORKDIR /src
COPY go/ go/
COPY rust/include/ rust/include/
COPY --from=rust-builder /src/rust/target/release/libschemar_core.so /src/rust/target/release/
RUN cd go && CGO_ENABLED=1 go build -o /out/schema-registry-server ./cmd/server

FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
ENV LD_LIBRARY_PATH=/usr/local/lib PYTHONPATH=/opt/schema-registry/python
COPY --from=rust-builder /src/rust/target/release/libschemar_core.so /usr/local/lib/
COPY --from=go-builder /out/schema-registry-server /usr/local/bin/schema-registry-server
COPY python/ /opt/schema-registry/python/
EXPOSE 8080
ENTRYPOINT ["schema-registry-server"]
CMD ["-addr", ":8080", "-store", "/var/lib/schema-registry/registry.json"]
