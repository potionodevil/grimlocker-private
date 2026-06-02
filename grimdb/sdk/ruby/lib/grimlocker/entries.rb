# frozen_string_literal: true
module Grimlocker
  Entry = Struct.new(:id, :title, :category, :fields, :created_at, :updated_at) do
    def self.from_hash(h)
      new(h['id']||'', h['title']||'', h['category']||'',
          h['fields']||{}, h['created_at']||0, h['updated_at']||0)
    end
    def field(key) = fields[key.to_s] || ''
  end

  PasswordEntry = Struct.new(:id, :title, :username, :password, :url, :notes) do
    def self.from_entry(e)
      new(e.id, e.title, e.field('username'), e.field('password'), e.field('url'), e.field('notes'))
    end
    def to_fields = { 'username' => username, 'password' => password, 'url' => url, 'notes' => notes }
  end

  SshKeyEntry = Struct.new(:id, :title, :public_key, :private_key, :username, :passphrase, :comment) do
    def self.from_entry(e)
      new(e.id, e.title, e.field('public_key'), e.field('private_key'),
          e.field('username'), e.field('passphrase'), e.field('comment'))
    end
    def to_fields = { 'public_key' => public_key, 'private_key' => private_key,
                      'username' => username, 'passphrase' => passphrase, 'comment' => comment }
  end

  CertificateEntry = Struct.new(:id, :title, :domain, :certificate, :private_key) do
    def self.from_entry(e)
      new(e.id, e.title, e.field('domain'), e.field('certificate'), e.field('private_key'))
    end
    def to_fields = { 'domain' => domain, 'certificate' => certificate, 'private_key' => private_key }
  end
end
