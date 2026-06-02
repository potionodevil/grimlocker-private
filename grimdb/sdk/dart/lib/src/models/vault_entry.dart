int _toInt(dynamic v) {
  if (v is int) return v;
  if (v is double) return v.toInt();
  if (v is String) return int.tryParse(v) ?? 0;
  return 0;
}

Map<String, String> _toStringMap(dynamic v) {
  if (v is Map) return v.map((k, e) => MapEntry(k.toString(), e.toString()));
  return {};
}

List<T> _toList<T>(dynamic v, T Function(Map<String, dynamic>) fromJson) {
  if (v is List) return v.map((e) => fromJson(e as Map<String, dynamic>)).toList();
  return [];
}

class VaultEntry {
  final String id;
  final String title;
  final String category;
  final Map<String, String> fields;
  final int createdAt;
  final int updatedAt;

  const VaultEntry({
    required this.id,
    required this.title,
    required this.category,
    this.fields = const {},
    required this.createdAt,
    required this.updatedAt,
  });

  factory VaultEntry.fromJson(Map<String, dynamic> json) => VaultEntry(
        id: json['id'] as String,
        title: json['title'] as String,
        category: json['category'] as String,
        fields: _toStringMap(json['fields']),
        createdAt: _toInt(json['created_at']),
        updatedAt: _toInt(json['updated_at']),
      );

  String field(String key) => fields[key] ?? '';
}

class PasswordEntry {
  String id;
  String title;
  String username;
  String password;
  String url;
  String notes;

  PasswordEntry({
    this.id = '',
    required this.title,
    this.username = '',
    this.password = '',
    this.url = '',
    this.notes = '',
  });

  factory PasswordEntry.fromEntry(VaultEntry e) => PasswordEntry(
        id: e.id,
        title: e.title,
        username: e.field('username'),
        password: e.field('password'),
        url: e.field('url'),
        notes: e.field('notes'),
      );

  Map<String, String> toFields() => {
        'username': username,
        'password': password,
        'url': url,
        'notes': notes,
      };
}

class SshKeyEntry {
  String id;
  String title;
  String publicKey;
  String privateKey;
  String username;
  String passphrase;

  SshKeyEntry({
    this.id = '',
    required this.title,
    this.publicKey = '',
    this.privateKey = '',
    this.username = '',
    this.passphrase = '',
  });

  factory SshKeyEntry.fromEntry(VaultEntry e) => SshKeyEntry(
        id: e.id,
        title: e.title,
        publicKey: e.field('public_key'),
        privateKey: e.field('private_key'),
        username: e.field('username'),
        passphrase: e.field('passphrase'),
      );

  Map<String, String> toFields() => {
        'public_key': publicKey,
        'private_key': privateKey,
        'username': username,
        'passphrase': passphrase,
      };
}

class CertificateEntry {
  String id;
  String title;
  String domain;
  String certificate;
  String privateKey;

  CertificateEntry({
    this.id = '',
    required this.title,
    this.domain = '',
    this.certificate = '',
    this.privateKey = '',
  });

  factory CertificateEntry.fromEntry(VaultEntry e) => CertificateEntry(
        id: e.id,
        title: e.title,
        domain: e.field('domain'),
        certificate: e.field('certificate'),
        privateKey: e.field('private_key'),
      );

  Map<String, String> toFields() => {
        'domain': domain,
        'certificate': certificate,
        'private_key': privateKey,
      };
}

class FileEntry {
  final String id;
  final String fileName;
  final String mimeType;
  final int totalSize;
  final String manifestBlockId;
  final String folderId;

  const FileEntry({
    required this.id,
    required this.fileName,
    required this.mimeType,
    required this.totalSize,
    required this.manifestBlockId,
    required this.folderId,
  });

  factory FileEntry.fromJson(Map<String, dynamic> json) => FileEntry(
        id: json['id'] as String,
        fileName: json['file_name'] as String,
        mimeType: json['mime_type'] as String,
        totalSize: _toInt(json['total_size']),
        manifestBlockId: json['manifest_block_id'] as String,
        folderId: json['folder_id'] as String,
      );
}

class FolderItem {
  final String id;
  final String name;
  final String type;

  const FolderItem({
    required this.id,
    required this.name,
    required this.type,
  });

  factory FolderItem.fromJson(Map<String, dynamic> json) => FolderItem(
        id: json['id'] as String,
        name: json['name'] as String,
        type: json['type'] as String,
      );
}

class FolderListing {
  final List<FolderItem> folders;
  final List<FileEntry> files;

  const FolderListing({
    this.folders = const [],
    this.files = const [],
  });

  factory FolderListing.fromJson(Map<String, dynamic> json) => FolderListing(
        folders: _toList(json['folders'], FolderItem.fromJson),
        files: _toList(json['files'], FileEntry.fromJson),
      );
}

class UploadProgress {
  final int bytesSent;
  final int totalBytes;

  const UploadProgress({required this.bytesSent, required this.totalBytes});

  double get percent =>
      totalBytes > 0 ? bytesSent * 100.0 / totalBytes : 100.0;
}

class Workspace {
  final String id;
  final String name;
  final bool isDefault;

  const Workspace({
    required this.id,
    required this.name,
    required this.isDefault,
  });

  factory Workspace.fromJson(Map<String, dynamic> json) => Workspace(
        id: json['id'] as String,
        name: json['name'] as String,
        isDefault: json['is_default'] as bool? ?? false,
      );
}

class SyncPeer {
  final String deviceId;
  final String host;
  final int port;
  final int seenAt;
  final bool reachable;

  const SyncPeer({
    required this.deviceId,
    required this.host,
    required this.port,
    required this.seenAt,
    required this.reachable,
  });

  factory SyncPeer.fromJson(Map<String, dynamic> json) => SyncPeer(
        deviceId: json['device_id'] as String,
        host: json['host'] as String,
        port: _toInt(json['port']),
        seenAt: _toInt(json['seen_at']),
        reachable: json['reachable'] as bool? ?? false,
      );
}

class SyncStatus {
  final List<SyncPeer> peers;
  final int lastSyncAt;
  final String deviceId;

  const SyncStatus({
    this.peers = const [],
    required this.lastSyncAt,
    required this.deviceId,
  });

  factory SyncStatus.fromJson(Map<String, dynamic> json) => SyncStatus(
        peers: _toList(json['peers'], SyncPeer.fromJson),
        lastSyncAt: _toInt(json['last_sync_at']),
        deviceId: json['device_id'] as String? ?? '',
      );
}

class AuditEvent {
  final int timestamp;
  final String level;
  final String module;
  final String message;
  final String? subjectId;

  const AuditEvent({
    required this.timestamp,
    required this.level,
    required this.module,
    required this.message,
    this.subjectId,
  });

  factory AuditEvent.fromJson(Map<String, dynamic> json) => AuditEvent(
        timestamp: _toInt(json['timestamp']),
        level: json['level'] as String,
        module: json['module'] as String,
        message: json['message'] as String,
        subjectId: json['subject_id'] as String?,
      );
}
