import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import '../grimlocker_exception.dart';
import '../models/vault_entry.dart';

class FileVaultClient {
  final String _endpoint;
  final Map<String, String> _headers;
  final http.Client _http;

  FileVaultClient(this._endpoint, this._headers, this._http);

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
