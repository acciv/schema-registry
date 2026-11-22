# schema-registry

Реестр версий API-схем. Проверяет, не сломала ли новая версия контракт.

Старая версия схемы хранится в реестре. Когда приходит новая, Rust-ядро сравнивает их и выдаёт список изменений. Если есть breaking change, сервер не регистрирует версию, если не передать `allow_breaking`.

## Зачем

Чтобы API менялся без сюрпризов: добавлять поля можно, менять типы и убирать обязательные поля — нельзя.

## Устройство

- **Rust** — сравнение схем и отпечатки. C ABI и CLI `schemar`.
- **Go** — HTTP-сервер реестра, файловое хранилище, счётчики.
- **Python** — CLI и клиент. Локальная проверка через ctypes.
- OpenAPI-спецификация, Docker, docker-compose, GitHub Actions.

## Правила совместимости

Новая схема должна принимать всё, что принимала старая:

- добавление необязательного поля — ок;
- добавление обязательного поля — breaking;
- удаление обязательного поля — breaking;
- смена типа — breaking;
- `integer` → `number` — ок, расширение;
- удаление значения из `enum` — breaking;
- поле было необязательным и стало обязательным — breaking;
- если `required` убрали — ок.

Формат схемы — упрощённый JSON Schema: `type`, `properties`, `required`, `items`, `enum`.

## Запуск

```bash
make test
make demo
```

Демо поднимает сервер, регистрирует три версии, показывает отказ на breaking change и принудительную регистрацию.

Вручную:

```bash
cargo build --release --manifest-path rust/Cargo.toml
cd go && go run ./cmd/server -addr :8080 -store registry.json
```

Docker:

```bash
docker compose up --build
curl http://127.0.0.1:8080/healthz
```

## CLI

Локально, без сервера:

```bash
PYTHONPATH=python python3 -m schemaregistry check old.json new.json
PYTHONPATH=python python3 -m schemaregistry fingerprint schema.json
```

Через сервер:

```bash
PYTHONPATH=python python3 -m schemaregistry --url http://127.0.0.1:8080 push user.created 1.0.0 schema.json
PYTHONPATH=python python3 -m schemaregistry --url http://127.0.0.1:8080 list
PYTHONPATH=python python3 -m schemaregistry --url http://127.0.0.1:8080 server-check user.created 1.0.0 new.json
```

Breaking change можно разрешить флагом `--force`.

## API

- `GET /healthz` — статус и версия ядра.
- `GET /stats` — регистрации, проверки, отказы.
- `GET /v1/schemas` — список схем.
- `POST /v1/schemas` — зарегистрировать версию.
- `GET /v1/schemas/{name}` — все версии.
- `GET /v1/schemas/{name}/{version}` — одна версия.
- `POST /v1/check` — сравнить две схемы.
- `POST /v1/schemas/{name}/{version}/check` — сравнить с сохранённой версией.

Полная спецификация: `docs/openapi.yaml`.

Пример ответа проверки:

```json
{
  "compatible": false,
  "breaking": 1,
  "changes": [
    {"path": "$.token", "kind": "property_added", "breaking": true, "to": "present"}
  ]
}
```

## Тесты

```bash
make test
```

Rust: 6 тестов. Go: 8 тестов. Python: 8 тестов.

Репозиторий: https://github.com/acciv/schema-registry

Сделал acciv.

<!-- linguist refresh -->
