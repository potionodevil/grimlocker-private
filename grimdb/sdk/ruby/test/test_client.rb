# frozen_string_literal: true
require 'minitest/autorun'
require 'grimlocker'
require 'json'
require 'base64'

module Grimlocker
  # Stub HTTP transport that records calls and returns canned responses.
  class TestClient < Client
    attr_reader :calls

    def initialize(base_url: 'http://127.0.0.1:36353', token: 'test-token')
      @calls = []
    end

    def unlock_vault!(password)
      @calls << ['vault.unlock', { password: password }]
    end

    def lock_vault!
      @calls << ['vault.logout', {}]
    end

    def vault_status
      @calls << ['vault.status', {}]
      { 'initialized' => true, 'unlocked' => true, 'status' => 'ok' }
    end

    def list_entries(category: nil)
      action = category ? 'entry.query' : 'entry.list'
      payload = category ? { category: category } : {}
      @calls << [action, payload]
      [
        Entry.new(id: 'e1', category: 'PASSWORD', title: 'Entry One',
                  fields: { 'username' => 'alice' }, created_at: 1, updated_at: 2),
        Entry.new(id: 'e2', category: 'SSH_KEY', title: 'Entry Two',
                  fields: {}, created_at: 3, updated_at: 4)
      ]
    end

    def get_entry(id)
      @calls << ['entry.read', { id: id }]
      raise GrimlockerError.new("Entry not found: #{id}", -10) if id == 'nonexistent'
      Entry.new(id: id, category: 'PASSWORD', title: 'Test Entry',
                fields: { 'username' => 'alice' }, created_at: 1, updated_at: 2)
    end

    def create_entry(title:, category:, fields: {})
      @calls << ['entry.create', { title: title, category: category, fields: fields }]
      Entry.new(id: 'new1', category: category, title: title,
                fields: fields, created_at: 10, updated_at: 20)
    end

    def update_entry!(id, fields:)
      @calls << ['entry.update', { id: id, fields: fields }]
    end

    def delete_entry!(id)
      @calls << ['entry.delete', { id: id }]
    end

    def search_entries(query, category: nil)
      payload = { query: query }
      payload[:category] = category if category
      @calls << ['entry.search', payload.]
      return [] if category == 'SSH_KEY'
      [Entry.new(id: 'e1', category: 'PASSWORD', title: 'GitHub',
                 fields: {}, created_at: 1, updated_at: 2)]
    end

    def passwords
      @calls << ['entry.query', { category: 'PASSWORD' }]
      [
        PasswordEntry.new(title: 'GitHub', username: 'alice', password: 'sec', url: '', notes: ''),
        PasswordEntry.new(title: 'GitLab', username: 'bob', password: 'sec', url: '', notes: '')
      ]
    end

    def create_password!(p)
      @calls << ['entry.create', { title: p.title, category: 'PASSWORD', fields: p.to_fields }]
      'p1'
    end

    def ssh_keys
      @calls << ['entry.query', { category: 'SSH_KEY' }]
      [SshKeyEntry.new(title: 'Key', public_key: 'pub', private_key: 'priv', username: '', passphrase: '', comment: '')]
    end

    def create_ssh_key!(k)
      @calls << ['entry.create', { title: k.title, category: 'SSH_KEY', fields: k.to_fields }]
      'sk1'
    end

    def certificates
      @calls << ['entry.query', { category: 'CERTIFICATE' }]
      [CertificateEntry.new(title: 'Cert', domain: 'ex.com', certificate: 'crt', private_key: 'key')]
    end

    def create_certificate!(c)
      @calls << ['entry.create', { title: c.title, category: 'CERTIFICATE', fields: c.to_fields }]
      'c1'
    end

    def list_folder(folder_id: '')
      @calls << ['file.list_folder', { folder_id: folder_id }]
      FolderListing.new(
        folders: [FolderItem.new('d1', 'sub', 'folder')],
        files: [FileEntry.new(id: 'f1', file_name: 'a.txt', mime_type: 'text/plain',
                               total_size: 10, manifest_block_id: 'mb1', folder_id: '')]
      )
    end

    def create_folder!(name, parent_id: '')
      @calls << ['file.create_folder', { name: name, parent_id: parent_id }]
      FolderItem.new('f1', name, 'folder')
    end

    def upload_file!(data, filename:, mime_type: 'application/octet-stream', folder_id: '', &block)
      yield({ bytes_sent: 0, total_bytes: data.bytesize }) if block
      @calls << ['file.ingest', { file_name: filename, mime_type: mime_type, folder_id: folder_id }]
      yield({ bytes_sent: data.bytesize, total_bytes: data.bytesize }) if block
      FileEntry.new(id: 'f1', file_name: filename, mime_type: mime_type,
                     total_size: data.bytesize, manifest_block_id: 'mb1', folder_id: folder_id)
    end

    def download_file(manifest_block_id)
      @calls << ['file.download', { manifest_block_id: manifest_block_id }]
      'hello'
    end

    def workspaces
      @calls << ['workspace.list', {}]
      [Workspace.new(id: 'ws1', name: 'Personal', is_default: true, created_at: 1)]
    end

    def create_workspace!(name)
      @calls << ['workspace.create', { name: name }]
      Workspace.new(id: 'ws2', name: name, is_default: false, created_at: 2)
    end

    def sync_status
      @calls << ['sync.list_peers', {}]
      SyncStatus.new(
        peers: [SyncPeer.new(device_id: 'd1', host: '192.168.1.5', port: 36352, seen_at: 1)],
        last_sync_at: 0, device_id: 'd1'
      )
    end

    def trigger_sync!
      @calls << ['sync.trigger', {}]
    end

    def audit_events(n: 50)
      @calls << ['audit.list', { n: n }]
      [AuditEvent.new(timestamp: 1, level: 'INFO', module: 'auth', message: 'unlock', subject_id: '')]
    end

    def health_check
      @calls << ['vault.status', {}]
      { 'status' => 'ok', 'daemon_version' => '1.0.0', 'vault_initialized' => true, 'vault_unlocked' => true }
    end

    def generate_ssh_key!(comment: '', save_to_vault: true)
      @calls << ['tool.ssh_keygen', { comment: comment, save_to_vault: save_to_vault }]
      { 'public_key' => 'ssh-ed25519 AAA', 'fingerprint' => 'SHA256:abc', 'entry_id' => 'e1' }
    end

    def recovery_phrase(password)
      @calls << ['vault.recovery_phrase', { password: password }]
      'abandon ability able about above absent absorb abstract...'
    end
  end

  class ErrorTestClient < Client
    def initialize
    end

    def list_entries(category: nil)
      raise GrimlockerError.new('vault is locked', -101)
    end

    def unlock_vault!(password)
      raise GrimlockerError.new('invalid password', -102)
    end
  end
end

class GrimlockerClientTest < Minitest::Test
  def setup
    @client = Grimlocker::TestClient.new
  end

  def test_unlock_vault
    @client.unlock_vault!('master-password')
    assert_equal(['vault.unlock', { password: 'master-password' }], @client.calls.first)
  end

  def test_lock_vault
    @client.lock_vault!
    assert_equal('vault.logout', @client.calls.first[0])
  end

  def test_vault_status
    status = @client.vault_status
    assert_equal(true, status['initialized'])
    assert_equal(true, status['unlocked'])
  end

  def test_list_entries
    entries = @client.list_entries
    assert_equal(2, entries.length)
    assert_equal('e1', entries[0].id)
    assert_equal('PASSWORD', entries[0].category)
  end

  def test_list_entries_by_category
    entries = @client.list_entries(category: 'PASSWORD')
    assert_equal(2, entries.length)
    assert_equal('entry.query', @client.calls.first[0])
  end

  def test_get_entry
    entry = @client.get_entry('e1')
    assert_equal('e1', entry.id)
    assert_equal('Test Entry', entry.title)
  end

  def test_get_entry_not_found
    assert_raises(Grimlocker::GrimlockerError) do
      @client.get_entry('nonexistent')
    end
  end

  def test_create_entry
    entry = @client.create_entry(title: 'New', category: 'PASSWORD', fields: { 'username' => 'alice' })
    assert_equal('new1', entry.id)
    assert_equal('PASSWORD', entry.category)
  end

  def test_update_entry
    @client.update_entry!('e1', fields: { 'notes' => 'updated' })
    assert_equal('entry.update', @client.calls.first[0])
    assert_equal('e1', @client.calls.first[1][:id])
  end

  def test_delete_entry
    @client.delete_entry!('e1')
    assert_equal('entry.delete', @client.calls.first[0])
  end

  def test_passwords
    passwords = @client.passwords
    assert_equal(2, passwords.length)
    assert_instance_of(Grimlocker::PasswordEntry, passwords[0])
    assert_equal('GitHub', passwords[0].title)
  end

  def test_create_password
    p = Grimlocker::PasswordEntry.new(title: 'GitHub', username: 'alice', password: 'sec', url: '', notes: '')
    id = @client.create_password!(p)
    assert_equal('p1', id)
  end

  def test_ssh_keys
    keys = @client.ssh_keys
    assert_equal(1, keys.length)
    assert_instance_of(Grimlocker::SshKeyEntry, keys[0])
  end

  def test_create_ssh_key
    k = Grimlocker::SshKeyEntry.new(title: 'Key', public_key: 'pub', private_key: 'priv', username: '', passphrase: '', comment: '')
    id = @client.create_ssh_key!(k)
    assert_equal('sk1', id)
  end

  def test_certificates
    certs = @client.certificates
    assert_equal(1, certs.length)
    assert_instance_of(Grimlocker::CertificateEntry, certs[0])
  end

  def test_create_certificate
    c = Grimlocker::CertificateEntry.new(title: 'Cert', domain: 'ex.com', certificate: 'crt', private_key: 'key')
    id = @client.create_certificate!(c)
    assert_equal('c1', id)
  end

  def test_search_entries
    results = @client.search_entries('git')
    assert_equal(1, results.length)
    assert_equal('e1', results[0].id)
  end

  def test_search_entries_with_category
    results = @client.search_entries('git', category: 'SSH_KEY')
    assert_equal(0, results.length)
  end

  def test_list_folder
    listing = @client.list_folder
    assert_equal(1, listing.folders.length)
    assert_equal(1, listing.files.length)
  end

  def test_create_folder
    folder = @client.create_folder!('Notes', parent_id: 'parent1')
    assert_equal('Notes', folder.name)
    assert_equal('f1', folder.id)
  end

  def test_upload_file
    progress_reports = []
    result = @client.upload_file!('hello', filename: 'doc.txt') do |p|
      progress_reports << p
    end
    assert_equal('doc.txt', result.file_name)
    assert_equal(2, progress_reports.length)
    assert_equal(0, progress_reports[0][:bytes_sent])
    assert_equal(5, progress_reports[1][:bytes_sent])
  end

  def test_download_file
    data = @client.download_file('mb1')
    assert_equal('hello', data)
  end

  def test_workspaces
    workspaces = @client.workspaces
    assert_equal(1, workspaces.length)
    assert_equal('Personal', workspaces[0].name)
  end

  def test_create_workspace
    ws = @client.create_workspace!('Work')
    assert_equal('Work', ws.name)
    assert_equal('ws2', ws.id)
  end

  def test_sync_status
    status = @client.sync_status
    assert_equal(1, status.peers.length)
    assert_equal('d1', status.device_id)
  end

  def test_trigger_sync
    @client.trigger_sync!
    assert_equal('sync.trigger', @client.calls.first[0])
  end

  def test_audit_events
    events = @client.audit_events(n: 10)
    assert_equal(1, events.length)
    assert_equal('INFO', events[0].level)
  end

  def test_health_check
    health = @client.health_check
    assert_equal('ok', health['status'])
  end

  def test_generate_ssh_key
    result = @client.generate_ssh_key!(comment: 'test', save_to_vault: true)
    assert(result.key?('public_key'))
    assert(result.key?('fingerprint'))
  end

  def test_recovery_phrase
    phrase = @client.recovery_phrase('master')
    assert(phrase.start_with?('abandon'))
  end

  def test_error_handling_locked
    client = Grimlocker::ErrorTestClient.new
    err = assert_raises(Grimlocker::GrimlockerError) do
      client.list_entries
    end
    assert_match(/locked/, err.message)
  end

  def test_error_handling_invalid_password
    client = Grimlocker::ErrorTestClient.new
    err = assert_raises(Grimlocker::GrimlockerError) do
      client.unlock_vault!('wrong')
    end
    assert_match(/invalid password/, err.message)
  end
end
