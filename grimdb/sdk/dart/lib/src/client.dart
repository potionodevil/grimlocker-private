import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'grimlocker_exception.dart';
import 'models/vault_entry.dart';

class GrimlockerClient {
  final String baseUrl;
  final String token;
  final http.Client _http;

  GrimlockerClient(String baseUrl, this.token)
      : _http = http.Client(),
        baseUrl = baseUrl.endsWith('/') ? baseUrl.substring(0, baseUrl.length - 1) : baseUrl;

  void close() => _http.close();

  String get _endpoint => '$baseUrl/api/v1';

  Map<String, String> get _headers => {
        'Content-Type': 'application/json',
        'X-Grimlocker-Token': token,
      };

  // ── Auth ──────────────────────────────────────────────────────────────────────

  Future<void> unlockVault(String password) async {
    await _call('vault.unlock', {'password': password});
  }

  Future<void> lockVault() async {
    await _call('vault.logout', {});
  }

  Future<Map<String, dynamic>> vaultStatus() async {
    return await _call('vault.status', {});
  }

  // ── Entries ───────────────────────────────────────────────────────────────────

  Future<List<VaultEntry>> listEntries({String? category}) async {
    final payload = category != null ? {'category': category} : <String, dynamic>{};
    final action = category != null ? 'entry.query' : 'entry.list';
    final result = await _call(action, payload);
    final list = result['entries'] ?? result;
    if (list is List) {
      return list.map((e) => VaultEntry.fromJson(e as Map<String, dynamic>)).toList();
    }
    return [];
  }

  Future<VaultEntry> getEntry(String id) async {
    final result = await _call('entry.read', {'id': id});
    return VaultEntry.fromJson(result);
  }

  Future<VaultEntry> createEntry(
      String title, String category, Map<String, String> fields) async {
    final result = await _call('entry.create', {
      'title': title,
      'category': category,
      'fields': fields,
    });
    return VaultEntry.fromJson(result);
  }

  Future<void> updateEntry(String id, Map<String, String> fields) async {
    await _call('entry.update', {'id': id, 'fields': fields});
  }

  Future<void> deleteEntry(String id) async {
    await _call('entry.delete', {'id': id});
  }

  Future<List<VaultEntry>> searchEntries(String query, {String? category}) async {
    final payload = <String, dynamic>{'query': query};
    if (category != null) {
      payload['category'] = category;
    }
    final result = await _call('entry.search', payload);
    if (result is List) {
      return result.map((e) => VaultEntry.fromJson(e as Map<String, dynamic>)).toList();
    }
    return [];
  }

  // ── Typed helpers ─────────────────────────────────────────────────────────────

  Future<List<PasswordEntry>> listPasswords() async {
    final entries = await listEntries(category: 'PASSWORD');
    return entries.map((e) => PasswordEntry.fromEntry(e)).toList();
  }

  Future<String> createPassword(PasswordEntry p) async {
    final entry = await createEntry(p.title, 'PASSWORD', p.toFields());
    return entry.id;
  }

  Future<List<SshKeyEntry>> listSshKeys() async {
    final entries = await listEntries(category: 'SSH_KEY');
    return entries.map((e) => SshKeyEntry.fromEntry(e)).toList();
  }

  Future<String> createSshKey(SshKeyEntry k) async {
    final entry = await createEntry(k.title, 'SSH_KEY', k.toFields());
    return entry.id;
  }

  Future<List<CertificateEntry>> listCertificates() async {
    final entries = await listEntries(category: 'CERTIFICATE');
    return entries.map((e) => CertificateEntry.fromEntry(e)).toList();
  }

  Future<String> createCertificate(CertificateEntry c) async {
    final entry = await createEntry(c.title, 'CERTIFICATE', c.toFields());
    return entry.id;
  }

  // ── File Vault ────────────────────────────────────────────────────────────────

  Future<FolderListing> listFolder({String folderId = ''}) async {
    final result = await _call('file.list_folder', {'folder_id': folderId});
    return FolderListing.fromJson(result);
  }

  Future<FolderItem> createFolder(String name, {String parentId = ''}) async {
    final result = await _call(
        'file.create_folder', {'name': name, 'parent_id': parentId});
    return FolderItem.fromJson(result);
  }

  Future<void> renameFolder(String id, String name) async {
    await _call('file.rename_folder', {'id': id, 'name': name});
  }

  Future<void> deleteFolder(String id) async {
    await _call('file.delete_folder', {'id': id});
  }

  Future<void> moveFile(String manifestBlockId, String folderId) async {
    await _call('file.move', {
      'manifest_block_id': manifestBlockId,
      'folder_id': folderId,
    });
  }

  Future<FileEntry> uploadFile(
    Uint8List data,
    String fileName, {
    String mimeType = 'application/octet-stream',
    String folderId = '',
    void Function(UploadProgress)? onProgress,
  }) async {
    onProgress?.call(UploadProgress(bytesSent: 0, totalBytes: data.length));
    final dataB64 = base64Encode(data);
    final result = await _call('file.ingest', {
      'file_name': fileName,
      'mime_type': mimeType,
      'folder_id': folderId,
      'data_b64': dataB64,
    });
    onProgress?.call(UploadProgress(bytesSent: data.length, totalBytes: data.length));
    return FileEntry.fromJson(result);
  }

  Future<Uint8List> downloadFile(String manifestBlockId) async {
    final result = await _call('file.download', {
      'manifest_block_id': manifestBlockId,
    });
    final dataB64 = result['data_b64'] as String?;
    if (dataB64 == null) {
      throw GrimlockerException(-10, 'Download returned no data');
    }
    return base64Decode(dataB64);
  }

  // ── Workspaces ────────────────────────────────────────────────────────────────

  Future<List<Workspace>> listWorkspaces() async {
    final result = await _call('workspace.list', {});
    if (result is List) {
      return result
          .map((e) => Workspace.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  }

  Future<Workspace> createWorkspace(String name) async {
    final result = await _call('workspace.create', {'name': name});
    return Workspace.fromJson(result);
  }

  Future<void> switchWorkspace(String id) async {
    await _call('workspace.switch', {'id': id});
  }

  Future<void> renameWorkspace(String id, String name) async {
    await _call('workspace.rename', {'id': id, 'name': name});
  }

  Future<void> deleteWorkspace(String id) async {
    await _call('workspace.delete', {'id': id});
  }

  // ── Sync ──────────────────────────────────────────────────────────────────────

  Future<SyncStatus> listSyncPeers() async {
    final result = await _call('sync.list_peers', {});
    return SyncStatus.fromJson(result);
  }

  Future<void> triggerSync() async {
    await _call('sync.trigger', {});
  }

  // ── Audit ─────────────────────────────────────────────────────────────────────

  Future<List<AuditEvent>> listAuditEvents({int n = 50}) async {
    final result = await _call('audit.list', {'n': n});
    final list = result['events'] ?? result;
    if (list is List) {
      return list
          .map((e) => AuditEvent.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  }

  // ── Health ────────────────────────────────────────────────────────────────────

  Future<Map<String, dynamic>> healthCheck() async {
    return await vaultStatus();
  }

  // ── Internal ──────────────────────────────────────────────────────────────────

  Future<dynamic> _call(
      String action, Map<String, dynamic> payload) async {
    final body = jsonEncode({'action': action, 'payload': payload});
    final response = await _http.post(
      Uri.parse(_endpoint),
      headers: _headers,
      body: body,
    );

    final responseBody = response.body;
    dynamic json;

    try {
      json = jsonDecode(responseBody);
    } catch (_) {
      json = {'error': responseBody};
    }

    if (response.statusCode < 200 || response.statusCode >= 300) {
      final map = json is Map<String, dynamic> ? json : <String, dynamic>{};
      final code = map['error_code'] as int? ?? 0;
      final msg = map['error'] as String? ?? responseBody;
      throw GrimlockerException(
          code, '${GrimlockerException.nameOf(code)} ($action): $msg');
    }

    if (json is Map<String, dynamic>) return json;
    if (json is List) return json;
    return <String, dynamic>{};
  }
}
