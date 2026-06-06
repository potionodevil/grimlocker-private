import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:http/http.dart' as http;

import 'file_vault/file_vault_client.dart';
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

  // ── Circuit breaker state ─────────────────────────────────────────────────────

  int _consecutiveFailures = 0;
  DateTime? _circuitOpenUntil;
  bool _isProbe = false;

  void _onFailure() {
    if (_isProbe) {
      _circuitOpenUntil = DateTime.now().add(const Duration(seconds: 30));
      _isProbe = false;
      return;
    }
    _consecutiveFailures++;
    if (_consecutiveFailures >= 5) {
      _circuitOpenUntil = DateTime.now().add(const Duration(seconds: 30));
      _consecutiveFailures = 0;
    }
  }

  void _onSuccess() {
    _consecutiveFailures = 0;
    _circuitOpenUntil = null;
    _isProbe = false;
  }

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

  Future<String> getRecoveryPhrase(String password) async {
    final result = await _call('vault.recovery_phrase', {'password': password});
    return result['recovery_phrase'] as String? ?? result['phrase'] as String? ?? '';
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

  Future<List<String>> createEntriesBatch(List<Map<String, dynamic>> entries) async {
    final futures = entries.map((e) async {
      final title = e['title'] as String;
      final category = e['category'] as String;
      final fields = (e['fields'] as Map).cast<String, String>();
      final entry = await createEntry(title, category, fields);
      return entry.id;
    });
    return await Future.wait(futures);
  }

  Future<void> deleteEntriesBatch(List<String> ids) async {
    await Future.wait(ids.map((id) => deleteEntry(id)));
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

  Future<Map<String, dynamic>> generateSSHKey({String comment = '', bool saveToVault = true}) async {
    return await _call('tool.ssh_keygen', {'comment': comment, 'save_to_vault': saveToVault});
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

  late final FileVaultClient fileVault = FileVaultClient(_endpoint, _headers, _http);

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
    if (_circuitOpenUntil != null) {
      if (DateTime.now().isBefore(_circuitOpenUntil!)) {
        throw CircuitBreakerOpenException();
      }
      _isProbe = true;
    }

    int attempt = 0;
    int delayMs = 100;
    Exception? lastException;

    while (true) {
      try {
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

        if (response.statusCode >= 400 && response.statusCode < 500) {
          final map = json is Map<String, dynamic> ? json : <String, dynamic>{};
          final code = map['error_code'] as int? ?? 0;
          final msg = map['error'] as String? ?? responseBody;
          throw GrimlockerException(
              code, '${GrimlockerException.nameOf(code)} ($action): $msg');
        }

        if (response.statusCode < 200 || response.statusCode >= 300) {
          final map = json is Map<String, dynamic> ? json : <String, dynamic>{};
          final code = map['error_code'] as int? ?? 0;
          final msg = map['error'] as String? ?? responseBody;
          lastException = GrimlockerException(
              code, '${GrimlockerException.nameOf(code)} ($action): $msg');
        } else {
          _onSuccess();
          if (json is Map<String, dynamic>) return json;
          if (json is List) return json;
          return <String, dynamic>{};
        }
      } on GrimlockerException {
        _onFailure();
        rethrow;
      } on http.ClientException catch (e) {
        lastException = e;
      } on TimeoutException catch (e) {
        lastException = e;
      } catch (e) {
        _onFailure();
        rethrow;
      }

      if (attempt >= 3) break;
      await Future.delayed(Duration(milliseconds: min(delayMs, 2000)));
      delayMs *= 2;
      attempt++;
    }

    _onFailure();
    throw lastException ?? Exception('Request failed after retries');
  }
}
