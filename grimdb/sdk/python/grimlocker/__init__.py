"""Grimlocker Python SDK — high-level client for the vault daemon."""

from grimlocker.client import Client, GrimlockerError
from grimlocker.entries import CertificateEntry, Entry, PasswordEntry, SSHKeyEntry
from grimlocker.files import FileEntry, FolderItem, FolderListing, UploadProgress

__all__ = [
    "Client",
    "GrimlockerError",
    "Entry",
    "PasswordEntry",
    "SSHKeyEntry",
    "CertificateEntry",
    "FileEntry",
    "FolderItem",
    "FolderListing",
    "UploadProgress",
]
__version__ = "1.0.0"
