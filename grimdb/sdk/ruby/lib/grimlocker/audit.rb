# frozen_string_literal: true
module Grimlocker
  AuditEvent = Struct.new(:timestamp, :level, :module, :message, :subject_id) do
    def self.from_hash(h) = new(h['timestamp']||0, h['level']||'', h['module']||'', h['message']||'', h['subject_id'])
  end
end
