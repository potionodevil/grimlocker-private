# frozen_string_literal: true
module Grimlocker
  Workspace = Struct.new(:id, :name, :is_default) do
    def self.from_hash(h) = new(h['id']||'', h['name']||'', h['is_default']||false)
  end
end
