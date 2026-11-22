"""CLI: python -m schemaregistry <command>"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

from .client import Client, RegistryError
from .core import VERSION, check, fingerprint


def load_json(path: str) -> dict[str, Any]:
    try:
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise SystemExit(f"не удалось прочитать {path}: {error}") from error


def print_json(value: Any) -> None:
    print(json.dumps(value, ensure_ascii=False, indent=2))


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="schema-registry CLI")
    parser.add_argument("--url", default="http://127.0.0.1:8080", help="адрес сервера")
    parser.add_argument("--version", action="version", version=VERSION)
    sub = parser.add_subparsers(dest="command", required=True)

    local_check = sub.add_parser("check", help="сравнить два файла локально")
    local_check.add_argument("old")
    local_check.add_argument("new")

    fp = sub.add_parser("fingerprint", help="отпечаток схемы")
    fp.add_argument("file")

    push = sub.add_parser("push", help="зарегистрировать версию схемы")
    push.add_argument("name")
    push.add_argument("version")
    push.add_argument("file")
    push.add_argument("--force", action="store_true", help="разрешить breaking changes")

    sub.add_parser("list", help="список схем")
    versions = sub.add_parser("versions", help="версии схемы")
    versions.add_argument("name")
    get = sub.add_parser("get", help="получить версию")
    get.add_argument("name")
    get.add_argument("version")

    remote_check = sub.add_parser("remote-check", help="сравнить через сервер")
    remote_check.add_argument("old")
    remote_check.add_argument("new")

    server_check = sub.add_parser("server-check", help="сравнить с сохранённой версией")
    server_check.add_argument("name")
    server_check.add_argument("version")
    server_check.add_argument("file")

    sub.add_parser("health", help="статус сервера")
    sub.add_parser("stats", help="счётчики сервера")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    client = Client(args.url)

    if args.command == "check":
        report = check(load_json(args.old), load_json(args.new))
        print_json(report)
        return 0 if report["compatible"] else 1
    if args.command == "fingerprint":
        print(fingerprint(load_json(args.file)))
        return 0

    try:
        if args.command == "push":
            result = client.push(args.name, args.version, load_json(args.file), args.force)
            print_json(result)
        elif args.command == "list":
            print_json(client.list_schemas())
        elif args.command == "versions":
            print_json(client.versions(args.name))
        elif args.command == "get":
            print_json(client.get(args.name, args.version))
        elif args.command == "remote-check":
            result = client.check(load_json(args.old), load_json(args.new))
            print_json(result)
            return 0 if result["compatible"] else 1
        elif args.command == "server-check":
            result = client.check_version(args.name, args.version, load_json(args.file))
            print_json(result)
            return 0 if result["compatible"] else 1
        elif args.command == "health":
            print_json(client.health())
        elif args.command == "stats":
            print_json(client.stats())
    except RegistryError as error:
        print(f"schema-registry: {error}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
