import 'dart:convert';
import 'dart:typed_data';

import 'package:test/test.dart';
import 'package:grimlocker_sdk/grimlocker_sdk.dart';
import 'package:http/http.dart' as http;

class MockClient extends http.BaseClient {
  final Map<String, List<http.Response>> _responses = {};
  final List<http.Request> requests = [];

  void when(String action, http.Response response) {
    _responses.putIfAbsent(action, () => []);
    _responses[action]!.add(response);
  }

  void whenJson(String action, int status, Map<String, dynamic> body) {
    when(action, http.Response(jsonEncode(body), status, headers: {'content-type': 'application/json'}));
  }

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    requests.add(request as http.Request);
    final bodyStr = await request.finalize().then((r) => r.bytesToString());
    final decoded = jsonDecode(bodyStr) as Map<String, dynamic>;
    final action = decoded['action'] as String;

    final responses = _responses[action];
    final response = responses != null && responses.isNotEmpty
        ? responses.removeAt(0)
        : http.Response(jsonEncode({'error': 'no mock for $action'}), 500);

    return http.StreamedResponse(
      Stream.value(utf8.encode(response.body)),
      response.statusCode,
      headers: response.headers,
    );
  }
}

class TestGrimlockerClient extends GrimlockerClient {
  final MockClient mockHttp;

  TestGrimlockerClient(this.mockHttp, {String baseUrl = 'http://127.0.0.1:36353', String token = 'test-token'})
      : super(baseUrl, token);

  @override
  http.Client get _http => mockHttp;
}

void main() {
  late MockClient mockHttp;
  late TestGrimlockerClient client;

  setUp(() {
    mockHttp = MockClient();
    client = TestGrimlockerClient(mockHttp);
  });

  group('Auth', () {
    test('unlockVault sends correct payload', () async {
      mockHttp.whenJson('vault.unlock', 200, {'success': true});
      await client.unlockVault('master-password');
      expect(mockHttp.requests.length, 1);
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['action'], 'vault.unlock');
      expect(body['payload']['password'], 'master-password');
    });

    test('lockVault sends logout', () async {
      mockHttp.whenJson('vault.logout', 200, {'success': true});
      await client.lockVault();
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['action'], 'vault.logout');
    });

    test('vaultStatus returns status', () async {
      mockHttp.whenJson('vault.status', 200, {'initialized': true, 'unlocked': true, 'status': 'ok'});
      final status = await client.vaultStatus();
      expect(status['initialized'], true);
      expect(status['unlocked'], true);
    });
  });

  group('Entries', () {
    test('listEntries returns all entries', () async {
      mockHttp.whenJson('entry.list', 200, {
        'entries': [
          {'id': 'e1', 'title': 'Entry 1', 'category': 'PASSWORD', 'created_at': 1, 'updated_at': 2},
          {'id': 'e2', 'title': 'Entry 2', 'category': 'SSH_KEY', 'created_at': 3, 'updated_at': 4},
        ]
      });
      final entries = await client.listEntries();
      expect(entries.length, 2);
      expect(entries[0].id, 'e1');
      expect(entries[1].category, 'SSH_KEY');
    });

    test('listEntries with category', () async {
      mockHttp.whenJson('entry.query', 200, {'entries': []});
      await client.listEntries(category: 'PASSWORD');
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['action'], 'entry.query');
      expect(body['payload']['category'], 'PASSWORD');
    });

    test('getEntry returns single entry', () async {
      mockHttp.whenJson('entry.read', 200, {'id': 'e1', 'title': 'Test', 'category': 'PASSWORD'});
      final entry = await client.getEntry('e1');
      expect(entry.id, 'e1');
      expect(entry.title, 'Test');
    });

    test('createEntry creates and returns entry', () async {
      mockHttp.whenJson('entry.create', 200, {'id': 'new1', 'title': 'New', 'category': 'PASSWORD'});
      final entry = await client.createEntry('New', 'PASSWORD', {'username': 'alice'});
      expect(entry.id, 'new1');
    });

    test('updateEntry sends update', () async {
      mockHttp.whenJson('entry.update', 200, {'success': true});
      await client.updateEntry('e1', {'notes': 'updated'});
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['action'], 'entry.update');
      expect(body['payload']['id'], 'e1');
    });

    test('deleteEntry sends delete', () async {
      mockHttp.whenJson('entry.delete', 200, {'success': true});
      await client.deleteEntry('e1');
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['action'], 'entry.delete');
      expect(body['payload']['id'], 'e1');
    });

    test('createEntriesBatch creates entries and returns ids', () async {
      mockHttp.whenJson('entry.create', 200, {'id': 'new1', 'title': 'A', 'category': 'PASSWORD'});
      mockHttp.whenJson('entry.create', 200, {'id': 'new2', 'title': 'B', 'category': 'PASSWORD'});
      final ids = await client.createEntriesBatch([
        {'title': 'A', 'category': 'PASSWORD', 'fields': <String, String>{}},
        {'title': 'B', 'category': 'PASSWORD', 'fields': <String, String>{}},
      ]);
      expect(ids, ['new1', 'new2']);
    });

    test('deleteEntriesBatch deletes entries', () async {
      mockHttp.whenJson('entry.delete', 200, {'success': true});
      mockHttp.whenJson('entry.delete', 200, {'success': true});
      await client.deleteEntriesBatch(['e1', 'e2']);
      final deleteRequests = mockHttp.requests.where((r) {
        final body = jsonDecode(r.body) as Map<String, dynamic>;
        return body['action'] == 'entry.delete';
      }).toList();
      expect(deleteRequests.length, 2);
    });

    test('searchEntries returns results', () async {
      mockHttp.whenJson('entry.search', 200, [
        {'id': 'e1', 'title': 'GitHub', 'category': 'PASSWORD'},
      ]);
      final results = await client.searchEntries('git');
      expect(results.length, 1);
      expect(results[0].title, 'GitHub');
    });

    test('searchEntries with category filter', () async {
      mockHttp.whenJson('entry.search', 200, []);
      await client.searchEntries('git', category: 'SSH_KEY');
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['payload']['category'], 'SSH_KEY');
    });
  });

  group('Typed helpers', () {
    test('listPasswords returns typed entries', () async {
      mockHttp.whenJson('entry.query', 200, {
        'entries': [
          {'id': 'p1', 'title': 'GitHub', 'category': 'PASSWORD', 'fields': {'username': 'a', 'password': 'b', 'url': '', 'notes': ''}}
        ]
      });
      final passwords = await client.listPasswords();
      expect(passwords.length, 1);
      expect(passwords[0].title, 'GitHub');
    });

    test('createPassword returns id', () async {
      mockHttp.whenJson('entry.create', 200, {'id': 'p1', 'title': 'GitHub', 'category': 'PASSWORD'});
      final p = PasswordEntry('GitHub', 'alice', 'sec');
      final id = await client.createPassword(p);
      expect(id, 'p1');
    });

    test('listSshKeys returns keys', () async {
      mockHttp.whenJson('entry.query', 200, {
        'entries': [
          {'id': 'sk1', 'title': 'Key', 'category': 'SSH_KEY', 'fields': {'public_key': 'pk', 'private_key': '', 'username': '', 'passphrase': '', 'comment': ''}}
        ]
      });
      final keys = await client.listSshKeys();
      expect(keys.length, 1);
    });

    test('createSshKey returns id', () async {
      mockHttp.whenJson('entry.create', 200, {'id': 'sk1', 'title': 'Key', 'category': 'SSH_KEY'});
      final k = SshKeyEntry('Key', 'pk', 'priv');
      final id = await client.createSshKey(k);
      expect(id, 'sk1');
    });

    test('listCertificates returns certs', () async {
      mockHttp.whenJson('entry.query', 200, {
        'entries': [
          {'id': 'c1', 'title': 'Cert', 'category': 'CERTIFICATE', 'fields': {'domain': 'ex.com', 'certificate': 'crt', 'private_key': 'key'}}
        ]
      });
      final certs = await client.listCertificates();
      expect(certs.length, 1);
    });

    test('createCertificate returns id', () async {
      mockHttp.whenJson('entry.create', 200, {'id': 'c1', 'title': 'Cert', 'category': 'CERTIFICATE'});
      final c = CertificateEntry('Cert', 'ex.com', 'crt', 'key');
      final id = await client.createCertificate(c);
      expect(id, 'c1');
    });
  });

  group('File Vault', () {
    test('listFolder returns listing', () async {
      mockHttp.whenJson('file.list_folder', 200, {
        'folders': [{'id': 'd1', 'name': 'sub', 'type': 'folder'}],
        'files': [{'id': 'f1', 'file_name': 'a.txt', 'mime_type': 'text/plain', 'total_size': 10, 'manifest_block_id': 'mb1', 'folder_id': ''}]
      });
      final listing = await client.listFolder();
      expect(listing.folders.length, 1);
      expect(listing.files.length, 1);
    });

    test('listFolder with id', () async {
      mockHttp.whenJson('file.list_folder', 200, {'folders': [], 'files': []});
      await client.listFolder('folder1');
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['payload']['folder_id'], 'folder1');
    });

    test('createFolder creates folder', () async {
      mockHttp.whenJson('file.create_folder', 200, {'id': 'f1', 'name': 'Notes'});
      final folder = await client.createFolder('Notes', 'parent1');
      expect(folder.name, 'Notes');
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['payload']['parent_id'], 'parent1');
    });

    test('uploadFile uploads and returns entry', () async {
      mockHttp.whenJson('file.ingest', 200, {
        'id': 'f1', 'file_name': 'doc.txt', 'mime_type': 'text/plain',
        'total_size': 11, 'manifest_block_id': 'mb1', 'folder_id': ''
      });
      final result = await client.uploadFile(utf8.encode('hello world'), 'doc.txt', 'text/plain');
      expect(result.fileName, 'doc.txt');
      expect(result.totalSize, 11);
    });

    test('downloadFile returns data', () async {
      final b64 = base64Encode(utf8.encode('hello'));
      mockHttp.whenJson('file.download', 200, {'data_b64': b64});
      final data = await client.downloadFile('mb1');
      expect(utf8.decode(data), 'hello');
    });

    test('moveFile sends move', () async {
      mockHttp.whenJson('file.move', 200, {'success': true});
      await client.moveFile('mb1', 'folder1');
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['action'], 'file.move');
      expect(body['payload']['manifest_block_id'], 'mb1');
      expect(body['payload']['folder_id'], 'folder1');
    });
  });

  group('Workspaces', () {
    test('listWorkspaces returns workspaces', () async {
      mockHttp.whenJson('workspace.list', 200, [
        {'id': 'ws1', 'name': 'Personal', 'is_default': true}
      ]);
      final workspaces = await client.listWorkspaces();
      expect(workspaces.length, 1);
      expect(workspaces[0].name, 'Personal');
    });

    test('createWorkspace creates workspace', () async {
      mockHttp.whenJson('workspace.create', 200, {'id': 'ws2', 'name': 'Work'});
      final ws = await client.createWorkspace('Work');
      expect(ws.name, 'Work');
    });
  });

  group('Sync', () {
    test('listSyncPeers returns status', () async {
      mockHttp.whenJson('sync.list_peers', 200, {
        'peers': [{'device_id': 'd1', 'host': '192.168.1.5', 'port': 36352, 'seen_at': 1}],
        'last_sync_at': 0, 'device_id': 'd1'
      });
      final status = await client.listSyncPeers();
      expect(status.peers.length, 1);
    });

    test('triggerSync sends trigger', () async {
      mockHttp.whenJson('sync.trigger', 200, {'success': true});
      await client.triggerSync();
      final body = jsonDecode(mockHttp.requests[0].body) as Map<String, dynamic>;
      expect(body['action'], 'sync.trigger');
    });
  });

  group('Audit', () {
    test('listAuditEvents returns events', () async {
      mockHttp.whenJson('audit.list', 200, {
        'events': [{'timestamp': 1, 'level': 'INFO', 'module': 'auth', 'message': 'unlock', 'subject_id': ''}]
      });
      final events = await client.listAuditEvents(10);
      expect(events.length, 1);
      expect(events[0].level, 'INFO');
    });
  });

  group('Health + Tools', () {
    test('healthCheck returns health status', () async {
      mockHttp.whenJson('vault.status', 200, {
        'status': 'ok', 'daemon_version': '1.0.0',
        'vault_initialized': true, 'vault_unlocked': true
      });
      final health = await client.healthCheck();
      expect(health.status, 'ok');
      expect(health.vaultInitialized, true);
    });

    test('generateSSHKey returns key result', () async {
      mockHttp.whenJson('tool.ssh_keygen', 200, {
        'public_key': 'ssh-ed25519 AAA...', 'fingerprint': 'SHA256:abc', 'entry_id': 'e1'
      });
      final result = await client.generateSSHKey('test', true);
      expect(result.publicKey, contains('ssh-ed25519'));
    });

    test('getRecoveryPhrase returns phrase', () async {
      mockHttp.whenJson('vault.recovery_phrase', 200, {
        'recovery_phrase': 'abandon ability able about above...'
      });
      final phrase = await client.getRecoveryPhrase('master');
      expect(phrase, 'abandon ability able about above...');
    });
  });

  group('Error handling', () {
    test('throws GrimlockerException on non-ok response', () async {
      mockHttp.whenJson('entry.list', 500, {'error': 'Internal Server Error'});
      expect(() => client.listEntries(), throwsA(isA<GrimlockerException>()));
    });

    test('throws on 401 unauthorized', () async {
      mockHttp.whenJson('entry.list', 401, {'error': 'Unauthorized'});
      expect(() => client.listEntries(), throwsA(isA<GrimlockerException>()));
    });

    test('circuitBreakerOpens', () async {
      for (var i = 0; i < 5; i++) {
        mockHttp.whenJson('entry.list', 500, {'error': 'Internal Server Error'});
        try {
          await client.listEntries();
        } catch (_) {}
      }
      expect(() => client.listEntries(), throwsA(isA<CircuitBreakerOpenException>()));
    });
  });

  test('close disposes HTTP client', () {
    client.close();
  });
}
