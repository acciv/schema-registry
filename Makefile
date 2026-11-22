.PHONY: all rust go python test demo run clean

all: rust go

rust:
	cargo build --release --manifest-path rust/Cargo.toml

go: rust
	cd go && go build ./...

python: rust
	PYTHONPATH=python python3 -m compileall -q python/schemaregistry

test: rust
	cargo test --manifest-path rust/Cargo.toml
	cd go && go test ./...
	PYTHONPATH=python python3 -m unittest discover -s python/tests -v

run: all
	cd go && go run ./cmd/server -addr :8080 -store registry.json

demo: all
	bash examples/demo.sh

clean:
	cargo clean --manifest-path rust/Cargo.toml
	rm -f registry.json /tmp/schema-registry-server
