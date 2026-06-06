# frozen_string_literal: true
require 'net/http'
require 'json'
require 'uri'
require 'base64'

require_relative 'error'
require_relative 'entries'
require_relative 'workspace'
require_relative 'sync'
require_relative 'audit'
require_relative 'file_vault'
require_relative 'version'

module Grimlocker
  # Synchronous HTTP client for the Grimlocker daemon.
  #
  # @example
  #   client = Grimlocker::Client.new(base_url: 'http://127.0.0.1:36353', token: ENV['GRIMLOCKER_TOKEN'])
  #   client.unlock_vault!('master-password')
  #   client.passwords.each { |p| puts "#{p.title}: #{p.username}" }
  class Client
    MAX_RETRIES = 3
    BASE_DELAY_MS = 100
    CIRCUIT_FAILURE_THRESHOLD = 5
    CIRCUIT_OPEN_SECONDS = 30

    # @param base_url [String] e.g. "http://127.0.0.1:36353"
    # @param token    [String] GRIMLOCKER_TOKEN from daemon stdout
    # @param timeout  [Integer] request timeout in seconds
    def initialize(base_url:, token:, timeout: 30)
      @uri     = URI.parse(base_url.chomp('/'))
      @token   = token
      @timeout = timeout
      @http    = Net::HTTP.new(@uri.host, @uri.port)
      @http.read_timeout  = timeout
      @http.open_timeout  = timeout
      @http.use_ssl       = @uri.scheme == 'https'
      @failure_count = 0
      @circuit_open_until = nil
    end

    # ── Auth ────────────────────────────────────────────────────────────────

    def unlock_vault!(password)     = call!('vault.unlock', { password: password })
    def lock_vault!                 = call!('vault.logout', {})
    def vault_status                = call!('vault.status', {})
    def recovery_phrase(password)   = call!('vault.recovery_phrase', { password: password })
    def generate_ssh_key(comment: '', save_to_vault: true)
      call!('tool.ssh_keygen', { comment: comment, save_to_vault: save_to_vault })
    end

    # ── Entries ─────────────────────────────────────────────────────────────

    def list_entries(category: nil)
      action  = category ? 'entry.query' : 'entry.list'
      payload = category ? { category: category } : {}
      parse_entries(call!(action, payload))
    end

    def get_entry(id)
      entries = parse_entries(call!('entry.read', { id: id }))
      entries.first or raise GrimlockerError.new("Entry not found: #{id}", -10)
    end

    def create_entry(title:, category:, fields: {})
      parse_entries(call!('entry.create', { title: title, category: category, fields: fields })).first
    end

    def update_entry!(id, fields:)  = call!('entry.update', { id: id, fields: fields })
    def delete_entry!(id)           = call!('entry.delete', { id: id })

    def search_entries(query, category: nil)
      parse_entries(call!('entry.search', { query: query, category: category }.compact))
    end

    def create_entries_batch(entries)
      entries.map do |entry|
        title    = entry[:title]    || entry['title']
        category = entry[:category] || entry['category']
        fields   = entry[:fields]   || entry['fields'] || {}
        create_entry(title: title, category: category, fields: fields).id
      end
    end

    def delete_entries_batch(ids)
      ids.each { |id| delete_entry!(id) }
      true
    end

    # ── Typed helpers ────────────────────────────────────────────────────────

    def passwords
      list_entries(category: 'PASSWORD').map { |e| PasswordEntry.from_entry(e) }
    end

    def create_password!(p)
      create_entry(title: p.title, category: 'PASSWORD', fields: p.to_fields).id
    end

    def ssh_keys
      list_entries(category: 'SSH_KEY').map { |e| SshKeyEntry.from_entry(e) }
    end

    def create_ssh_key!(k)
      create_entry(title: k.title, category: 'SSH_KEY', fields: k.to_fields).id
    end

    def certificates
      list_entries(category: 'CERTIFICATE').map { |e| CertificateEntry.from_entry(e) }
    end

    def create_certificate!(c)
      create_entry(title: c.title, category: 'CERTIFICATE', fields: c.to_fields).id
    end

    # ── File Vault ───────────────────────────────────────────────────────────

    def list_folder(folder_id: '')
      FolderListing.from_hash(call!('file.list_folder', { folder_id: folder_id }))
    end

    def create_folder!(name, parent_id: '')
      r = call!('file.create_folder', { name: name, parent_id: parent_id })
      FolderItem.new(r['id'] || '', name, 'folder')
    end

    def rename_folder!(id, name)  = call!('file.rename_folder', { id: id, name: name })
    def delete_folder!(id)        = call!('file.delete_folder', { id: id })
    def move_file!(manifest_block_id, folder_id:)
      call!('file.move', { manifest_block_id: manifest_block_id, folder_id: folder_id })
    end

    # @yield [progress] Progress hash with :bytes_sent and :total_bytes
    def upload_file!(data, filename:, mime_type: 'application/octet-stream', folder_id: '')
      yield({ bytes_sent: 0, total_bytes: data.bytesize }) if block_given?
      r = call!('file.ingest', {
        file_name: filename, mime_type: mime_type,
        folder_id: folder_id, data_b64: Base64.strict_encode64(data)
      })
      yield({ bytes_sent: data.bytesize, total_bytes: data.bytesize }) if block_given?
      FileEntry.from_hash(r)
    end

    def download_file(manifest_block_id)
      r = call!('file.download', { manifest_block_id: manifest_block_id })
      Base64.decode64(r['data_b64'] || '')
    end

    # ── Workspaces ───────────────────────────────────────────────────────────

    def workspaces
      Array(call!('workspace.list', {})).map { |w| Workspace.from_hash(w) }
    end

    def create_workspace!(name)
      Workspace.from_hash(call!('workspace.create', { name: name }))
    end

    def switch_workspace!(id)      = call!('workspace.switch', { id: id })
    def rename_workspace!(id, name) = call!('workspace.rename', { id: id, name: name })
    def delete_workspace!(id)       = call!('workspace.delete', { id: id })

    # ── Sync ────────────────────────────────────────────────────────────────

    def sync_status
      SyncStatus.from_hash(call!('sync.list_peers', {}))
    end

    def trigger_sync! = call!('sync.trigger', {})

    # ── Audit ────────────────────────────────────────────────────────────────

    def audit_events(n: 50)
      Array(call!('audit.list', { n: n })).map { |e| AuditEvent.from_hash(e) }
    end

    # ── Health ───────────────────────────────────────────────────────────────

    def health_check = vault_status

    private

    def call!(action, payload)
      now = Time.now.to_f

      if @circuit_open_until
        if now < @circuit_open_until
          raise CircuitBreakerOpenError.new('Circuit breaker is open')
        end
        @circuit_open_until = nil
      end

      last_error = nil

      (0..MAX_RETRIES).each do |attempt|
        begin
          req = Net::HTTP::Post.new('/api/v1')
          req['Content-Type']       = 'application/json'
          req['X-Grimlocker-Token'] = @token
          req.body = JSON.generate({ action: action, payload: payload })

          res = @http.request(req)
          body = JSON.parse(res.body)

          unless res.is_a?(Net::HTTPSuccess)
            code = body['error_code'] || 0
            msg  = body['error'] || "HTTP #{res.code}"
            last_error = GrimlockerError.new("#{GrimlockerError.name_of(code)}: #{msg}", code)

            status = res.code.to_i

            if status >= 400 && status < 500
              @failure_count = 0
              raise last_error
            end

            if status >= 500
              record_failure
              if attempt == MAX_RETRIES
                raise last_error
              end
              delay = [BASE_DELAY_MS * (1 << attempt), 2000].min
              sleep(delay / 1000.0)
              next
            end

            raise last_error
          end

          @failure_count = 0
          return body
        rescue JSON::ParserError => e
          record_failure
          last_error = GrimlockerError.new("Invalid JSON response: #{e.message}")
          if attempt == MAX_RETRIES
            raise last_error
          end
          delay = [BASE_DELAY_MS * (1 << attempt), 2000].min
          sleep(delay / 1000.0)
        rescue Errno::ECONNREFUSED, Errno::ETIMEDOUT, Errno::ECONNRESET, SocketError, Net::OpenTimeout, Net::ReadTimeout, EOFError => e
          record_failure
          last_error = GrimlockerError.new("Connection failed: #{e.message}")
          if attempt == MAX_RETRIES
            raise last_error
          end
          delay = [BASE_DELAY_MS * (1 << attempt), 2000].min
          sleep(delay / 1000.0)
        end
      end

      raise last_error || GrimlockerError.new('Request failed after retries')
    end

    def record_failure
      @failure_count += 1
      if @failure_count >= CIRCUIT_FAILURE_THRESHOLD
        @circuit_open_until = Time.now.to_f + CIRCUIT_OPEN_SECONDS
      end
    end

    def parse_entries(data)
      arr = data.is_a?(Array) ? data : Array(data['entries'])
      arr.map { |e| Entry.from_hash(e) }
    end
  end
end
