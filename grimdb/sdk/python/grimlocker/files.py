"""File vault helpers for the Grimlocker Python SDK."""

from dataclasses import dataclass, field
from typing import List, Optional, Callable


@dataclass
class FileEntry:
    id: str = ""
    file_name: str = ""
    mime_type: str = ""
    total_size: int = 0
    manifest_block_id: str = ""
    folder_id: str = ""


@dataclass
class FolderItem:
    id: str = ""
    name: str = ""
    type: str = ""


@dataclass
class FolderListing:
    folders: List[FolderItem] = field(default_factory=list)
    files: List[FileEntry] = field(default_factory=list)


@dataclass
class UploadProgress:
    bytes_sent: int = 0
    total_bytes: int = 0
