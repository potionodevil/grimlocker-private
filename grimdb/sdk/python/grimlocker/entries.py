"""Typed domain objects for vault entries."""

from dataclasses import dataclass, field
from typing import Dict


@dataclass
class Entry:
    id: str = ""
    category: str = ""
    title: str = ""
    fields: Dict[str, str] = field(default_factory=dict)
    created_at: int = 0
    updated_at: int = 0

    @classmethod
    def from_dict(cls, d: dict) -> "Entry":
        return cls(
            id=d.get("id", ""),
            category=d.get("category", ""),
            title=d.get("title", ""),
            fields=d.get("fields") or {},
            created_at=d.get("created_at", 0),
            updated_at=d.get("updated_at", 0),
        )


@dataclass
class PasswordEntry:
    title: str
    username: str = ""
    password: str = ""
    url: str = ""
    notes: str = ""
    id: str = ""

    @classmethod
    def from_entry(cls, e: Entry) -> "PasswordEntry":
        return cls(
            id=e.id,
            title=e.title,
            username=e.fields.get("username", ""),
            password=e.fields.get("password", ""),
            url=e.fields.get("url", ""),
            notes=e.fields.get("notes", ""),
        )

    def to_fields(self) -> Dict[str, str]:
        return {
            "username": self.username,
            "password": self.password,
            "url": self.url,
            "notes": self.notes,
        }


@dataclass
class SSHKeyEntry:
    title: str
    public_key: str = ""
    comment: str = ""
    algorithm: str = ""
    id: str = ""

    @classmethod
    def from_entry(cls, e: Entry) -> "SSHKeyEntry":
        return cls(
            id=e.id,
            title=e.title,
            public_key=e.fields.get("public_key", ""),
            comment=e.fields.get("comment", ""),
            algorithm=e.fields.get("algorithm", ""),
        )

    def to_fields(self) -> Dict[str, str]:
        return {
            "public_key": self.public_key,
            "comment": self.comment,
            "algorithm": self.algorithm,
        }


@dataclass
class CertificateEntry:
    title: str
    domain: str = ""
    certificate: str = ""
    private_key: str = ""
    id: str = ""

    @classmethod
    def from_entry(cls, e: Entry) -> "CertificateEntry":
        return cls(
            id=e.id,
            title=e.title,
            domain=e.fields.get("domain", ""),
            certificate=e.fields.get("certificate", ""),
            private_key=e.fields.get("private_key", ""),
        )

    def to_fields(self) -> Dict[str, str]:
        return {
            "domain": self.domain,
            "certificate": self.certificate,
            "private_key": self.private_key,
        }
