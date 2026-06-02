# frozen_string_literal: true
module Grimlocker
  SyncPeer = Struct.new(:device_id, :host, :port, :seen_at, :reachable) do
    def self.from_hash(h) = new(h['device_id']||'', h['host']||'', h['port']||0, h['seen_at']||0, h.fetch('reachable',true))
  end

  SyncStatus = Struct.new(:peers, :last_sync_at, :device_id) do
    def self.from_hash(h)
      new(Array(h['peers']).map { |p| SyncPeer.from_hash(p) }, h['last_sync_at']||0, h['device_id']||'')
    end
  end
end
