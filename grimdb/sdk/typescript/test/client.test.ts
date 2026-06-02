import { describe, it, expect, beforeEach, vi } from 'vitest';
import { GrimlockerClient } from '../src/index';

function createClient(baseUrl = 'http://127.0.0.1:36353', token = 'test-token') {
  return new GrimlockerClient(baseUrl, token);
}

function mockResponse(status: number, body: unknown) {
  (globalThis as any).fetch = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  });
}

function getLastCallBody() {
  const calls = (globalThis as any).fetch.mock.calls;
  if (!calls || calls.length === 0) return null;
  const lastCall = calls[calls.length - 1];
  const body = lastCall[1]?.body;
  return body ? JSON.parse(body) : null;
}

function getLastCallUrl() {
  const calls = (globalThis as any).fetch.mock.calls;
  if (!calls || calls.length === 0) return '';
  return calls[calls.length - 1][0];
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe('GrimlockerClient', () => {
  describe('constructor', () => {
    it('strips trailing slash from baseUrl', () => {
      const client = createClient('http://127.0.0.1:36353/', 'token');
      expect((client as any).baseUrl).toBe('http://127.0.0.1:36353');
    });
  });

  describe('unlockVault', () => {
    it('sends unlock request and emits connected event', async () => {
      mockResponse(200, { success: true });
      const client = createClient();
      const listener = vi.fn();
      client.on('connected', listener);

      await client.unlockVault('master-password');

      const body = getLastCallBody();
      expect(body.action).toBe('vault.unlock');
      expect(body.payload.password).toBe('master-password');
      expect(listener).toHaveBeenCalled();
    });
  });

  describe('lockVault', () => {
    it('sends logout request and emits disconnected event', async () => {
      mockResponse(200, { success: true });
      const client = createClient();
      const listener = vi.fn();
      client.on('disconnected', listener);

      await client.lockVault();

      const body = getLastCallBody();
      expect(body.action).toBe('vault.logout');
      expect(listener).toHaveBeenCalled();
    });
  });

  describe('vaultStatus', () => {
    it('returns vault status', async () => {
      mockResponse(200, { initialized: true, unlocked: true });
      const client = createClient();

      const status = await client.vaultStatus();

      expect(status.initialized).toBe(true);
      expect(status.unlocked).toBe(true);
    });
  });

  describe('listEntries', () => {
    it('lists all entries without category', async () => {
      mockResponse(200, {
        entries: [
          { id: 'e1', title: 'Entry 1', category: 'PASSWORD', created_at: 1, updated_at: 1 },
        ],
      });
      const client = createClient();

      const entries = await client.listEntries();

      expect(entries).toHaveLength(1);
      expect(entries[0].id).toBe('e1');
      const body = getLastCallBody();
      expect(body.action).toBe('entry.list');
    });

    it('lists entries filtered by category', async () => {
      mockResponse(200, { entries: [] });
      const client = createClient();

      await client.listEntries('SSH_KEY');

      const body = getLastCallBody();
      expect(body.action).toBe('entry.list');
      expect(body.payload.category).toBe('SSH_KEY');
    });
  });

  describe('getEntry', () => {
    it('fetches a single entry by id', async () => {
      mockResponse(200, { id: 'e1', title: 'Entry 1', category: 'PASSWORD' });
      const client = createClient();

      const entry = await client.getEntry('e1');

      expect(entry.id).toBe('e1');
      const body = getLastCallBody();
      expect(body.payload.id).toBe('e1');
      expect(body.action).toBe('entry.read');
    });
  });

  describe('createPassword', () => {
    it('creates a password entry', async () => {
      mockResponse(200, { id: 'p1', title: 'GitHub', category: 'PASSWORD' });
      const client = createClient();

      const entry = await client.createPassword('GitHub', 'alice', 's3cret', 'https://github.com', 'notes');

      expect(entry.id).toBe('p1');
      expect(entry.category).toBe('PASSWORD');
      const body = getLastCallBody();
      expect(body.payload.fields.username).toBe('alice');
      expect(body.payload.fields.password).toBe('s3cret');
    });
  });

  describe('createSSHKey', () => {
    it('creates an SSH key entry', async () => {
      mockResponse(200, { id: 'sk1', title: 'My Key', category: 'SSH_KEY' });
      const client = createClient();

      const entry = await client.createSSHKey('My Key', 'pubkey', 'privkey', 'alice');

      expect(entry.id).toBe('sk1');
      const body = getLastCallBody();
      expect(body.payload.fields.public_key).toBe('pubkey');
      expect(body.payload.fields.private_key).toBe('privkey');
    });
  });

  describe('createCertificate', () => {
    it('creates a certificate entry', async () => {
      mockResponse(200, { id: 'c1', title: 'My Cert', category: 'CERTIFICATE' });
      const client = createClient();

      const entry = await client.createCertificate('My Cert', 'example.com', 'cert-data', 'key-data');

      expect(entry.id).toBe('c1');
      const body = getLastCallBody();
      expect(body.payload.fields.domain).toBe('example.com');
      expect(body.payload.fields.certificate).toBe('cert-data');
    });
  });

  describe('updateEntry', () => {
    it('updates entry fields', async () => {
      mockResponse(200, { success: true });
      const client = createClient();

      await client.updateEntry('e1', { username: 'newuser' });

      const body = getLastCallBody();
      expect(body.action).toBe('entry.update');
      expect(body.payload.id).toBe('e1');
    });
  });

  describe('deleteEntry', () => {
    it('deletes an entry', async () => {
      mockResponse(200, { success: true });
      const client = createClient();

      await client.deleteEntry('e1');

      const body = getLastCallBody();
      expect(body.action).toBe('entry.delete');
      expect(body.payload.id).toBe('e1');
    });
  });

  describe('searchEntries', () => {
    it('searches entries by query', async () => {
      mockResponse(200, {
        entries: [{ id: 'e1', title: 'GitHub', category: 'PASSWORD' }],
      });
      const client = createClient();

      const results = await client.searchEntries('git');

      expect(results).toHaveLength(1);
      const body = getLastCallBody();
      expect(body.payload.query).toBe('git');
    });

    it('searches with category filter', async () => {
      mockResponse(200, { entries: [] });
      const client = createClient();

      await client.searchEntries('git', 'SSH_KEY');

      const body = getLastCallBody();
      expect(body.payload.category).toBe('SSH_KEY');
    });
  });

  describe('listCertificates', () => {
    it('lists certificate entries', async () => {
      mockResponse(200, {
        entries: [{ id: 'c1', title: 'Cert', category: 'CERTIFICATE', domain: 'example.com', certificate: '...' }],
      });
      const client = createClient();

      const certs = await client.listCertificates();

      expect(certs).toHaveLength(1);
    });
  });

  describe('uploadFile', () => {
    it('uploads a file and reports progress', async () => {
      mockResponse(200, { id: 'f1', file_name: 'test.txt', mime_type: 'text/plain', total_size: 11, manifest_block_id: 'mb1', folder_id: '' });
      const client = createClient();
      const onProgress = vi.fn();

      const result = await client.uploadFile('hello world', 'test.txt', 'text/plain', '', onProgress);

      expect(result.file_name).toBe('test.txt');
      expect(onProgress).toHaveBeenCalledTimes(2);
      expect(onProgress).toHaveBeenNthCalledWith(1, { bytes_sent: 0, total_bytes: 11 });
      expect(onProgress).toHaveBeenNthCalledWith(2, { bytes_sent: 11, total_bytes: 11 });
    });
  });

  describe('downloadFile', () => {
    it('downloads a file', async () => {
      const b64Data = Buffer.from('hello').toString('base64');
      mockResponse(200, { data_b64: b64Data });
      const client = createClient();

      const data = await client.downloadFile('mb1');

      const body = getLastCallBody();
      expect(body.action).toBe('file.download');
      expect(data).toBeInstanceOf(Uint8Array);
    });
  });

  describe('listFolder', () => {
    it('lists folder contents', async () => {
      mockResponse(200, {
        folders: [{ id: 'd1', name: 'sub', type: 'folder' }],
        files: [{ id: 'f1', file_name: 'a.txt', mime_type: 'text/plain', total_size: 10, manifest_block_id: 'mb1', folder_id: '' }],
      });
      const client = createClient();

      const listing = await client.listFolder('');

      expect(listing.folders).toHaveLength(1);
      expect(listing.files).toHaveLength(1);
    });

    it('lists folder by id', async () => {
      mockResponse(200, { folders: [], files: [] });
      const client = createClient();

      await client.listFolder('folder1');

      const body = getLastCallBody();
      expect(body.payload.folder_id).toBe('folder1');
    });
  });

  describe('createFolder', () => {
    it('creates a new folder', async () => {
      mockResponse(200, { id: 'f1', name: 'Notes', type: 'folder' });
      const client = createClient();

      const folder = await client.createFolder('Notes', 'parent1');

      expect(folder.name).toBe('Notes');
      const body = getLastCallBody();
      expect(body.payload.parent_id).toBe('parent1');
    });
  });

  describe('deleteFolder', () => {
    it('deletes a folder', async () => {
      mockResponse(200, { success: true });
      const client = createClient();

      await client.deleteFolder('f1');

      const body = getLastCallBody();
      expect(body.action).toBe('file.delete_folder');
      expect(body.payload.id).toBe('f1');
    });
  });

  describe('moveFile', () => {
    it('moves a file to a folder', async () => {
      mockResponse(200, { success: true });
      const client = createClient();

      await client.moveFile('mb1', 'folder1');

      const body = getLastCallBody();
      expect(body.action).toBe('file.move');
      expect(body.payload.manifest_block_id).toBe('mb1');
      expect(body.payload.folder_id).toBe('folder1');
    });
  });

  describe('listWorkspaces', () => {
    it('lists all workspaces', async () => {
      mockResponse(200, [{ id: 'ws1', name: 'Personal', is_default: true }]);
      const client = createClient();

      const workspaces = await client.listWorkspaces();

      expect(workspaces).toHaveLength(1);
      expect(workspaces[0].name).toBe('Personal');
      const body = getLastCallBody();
      expect(body.action).toBe('workspace.list');
    });
  });

  describe('createWorkspace', () => {
    it('creates a new workspace', async () => {
      mockResponse(200, { id: 'ws2', name: 'Work' });
      const client = createClient();

      const ws = await client.createWorkspace('Work');

      expect(ws.name).toBe('Work');
      const body = getLastCallBody();
      expect(body.action).toBe('workspace.create');
      expect(body.payload.name).toBe('Work');
    });
  });

  describe('switchWorkspace', () => {
    it('switches active workspace', async () => {
      mockResponse(200, { success: true });
      const client = createClient();

      await client.switchWorkspace('ws2');

      const body = getLastCallBody();
      expect(body.action).toBe('workspace.switch');
      expect(body.payload.id).toBe('ws2');
    });
  });

  describe('listSyncPeers', () => {
    it('lists sync peers', async () => {
      mockResponse(200, {
        peers: [{ device_id: 'd1', host: '192.168.1.5', port: 36352, seen_at: 1 }],
        last_sync_at: 0,
        device_id: 'd1',
      });
      const client = createClient();

      const status = await client.listSyncPeers();

      expect(status.peers).toHaveLength(1);
      const body = getLastCallBody();
      expect(body.action).toBe('sync.list_peers');
    });
  });

  describe('triggerSync', () => {
    it('triggers a sync cycle', async () => {
      mockResponse(200, { success: true });
      const client = createClient();

      await client.triggerSync();

      const body = getLastCallBody();
      expect(body.action).toBe('sync.trigger');
    });
  });

  describe('listAuditLog', () => {
    it('lists audit events', async () => {
      mockResponse(200, {
        events: [{ timestamp: 1, level: 'INFO', module: 'auth', message: 'unlock', subject_id: '' }],
      });
      const client = createClient();

      const events = await client.listAuditLog(10);

      expect(events).toHaveLength(1);
      expect(events[0].level).toBe('INFO');
    });
  });

  describe('healthCheck', () => {
    it('retrieves health status', async () => {
      mockResponse(200, { status: 'ok', daemon_version: '1.0.0', vault_initialized: true, vault_unlocked: true });
      const client = createClient();

      const health = await client.healthCheck();

      expect(health.status).toBe('ok');
      expect(health.vault_initialized).toBe(true);
    });
  });

  describe('generateSSHKey', () => {
    it('generates an SSH key pair', async () => {
      mockResponse(200, { public_key: 'ssh-ed25519 AAAAC3...', fingerprint: 'SHA256:abc', entry_id: 'e99' });
      const client = createClient();

      const result = await client.generateSSHKey('test', true);

      expect(result.public_key).toContain('ssh-ed25519');
      expect(result.fingerprint).toBe('SHA256:abc');
    });

    it('generates without saving to vault', async () => {
      mockResponse(200, { public_key: 'ssh-ed25519...', fingerprint: 'SHA256:xyz' });
      const client = createClient();

      await client.generateSSHKey('', false);

      const body = getLastCallBody();
      expect(body.payload.save_to_vault).toBe(false);
    });
  });

  describe('getRecoveryPhrase', () => {
    it('retrieves the recovery phrase', async () => {
      mockResponse(200, { recovery_phrase: 'abandon ability able about above...' });
      const client = createClient();

      const phrase = await client.getRecoveryPhrase('master');

      expect(phrase).toBe('abandon ability able about above...');
      const body = getLastCallBody();
      expect(body.payload.password).toBe('master');
    });
  });

  describe('event emission', () => {
    it('emits error event on failed request', async () => {
      mockResponse(500, { error: 'Internal Server Error' });
      const client = createClient();
      const errorListener = vi.fn();
      client.on('error', errorListener);

      await expect(client.listEntries()).rejects.toBeDefined();
      expect(errorListener).toHaveBeenCalled();
    });

    it('supports removing event listeners', () => {
      const client = createClient();
      const listener = vi.fn();
      client.on('connected', listener);
      client.off('connected', listener);
      expect((client as any)._listeners.get('connected')?.size).toBe(0);
    });

    it('swallows listener errors', async () => {
      mockResponse(200, { success: true });
      const client = createClient();
      const badListener = vi.fn().mockImplementation(() => { throw new Error('boom'); });
      client.on('connected', badListener);

      await client.unlockVault('pass');
    });
  });

  describe('error handling', () => {
    it('throws on non-ok response', async () => {
      mockResponse(401, { error: 'Unauthorized' });
      const client = createClient();

      await expect(client.listEntries()).rejects.toMatchObject({
        message: 'Unauthorized',
        status_code: 401,
      });
    });

    it('handles non-json error body', async () => {
      (globalThis as any).fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.reject(new Error('invalid json')),
        text: () => Promise.resolve('Server Error'),
      });
      const client = createClient();

      await expect(client.listEntries()).rejects.toMatchObject({
        status_code: 500,
      });
    });
  });
});
