# frozen_string_literal: true
module Grimlocker
  FolderItem = Struct.new(:id, :name, :kind)
  FileEntry = Struct.new(:id, :file_name, :mime_type, :total_size, :manifest_block_id, :folder_id) do
    def self.from_hash(h)
      new(h['id']||'', h['file_name']||'', h['mime_type']||'',
          h['total_size']||0, h['manifest_block_id']||'', h['folder_id']||'')
    end
  end
  FolderListing = Struct.new(:folders, :files) do
    def self.from_hash(h)
      folders = Array(h['folders']).map { |f| FolderItem.new(f['id']||'', f['name']||'', 'folder') }
      files   = Array(h['files']).map   { |f| FileEntry.from_hash(f) }
      new(folders, files)
    end
  end
end
