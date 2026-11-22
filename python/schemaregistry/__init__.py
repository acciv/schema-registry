"""schema-registry: клиент и локальные функции проверки API-схем."""

from .client import Client, RegistryError
from .core import VERSION, check, fingerprint

__all__ = ["Client", "RegistryError", "VERSION", "check", "fingerprint"]
