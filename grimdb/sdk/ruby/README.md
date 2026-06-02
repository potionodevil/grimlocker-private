# grimlocker (Ruby Gem)

Ruby SDK for the Grimlocker Zero-Trust Vault. Zero external dependencies — uses Ruby's stdlib `net/http`.

## Installation

```ruby
gem 'grimlocker'
```

## Quick Start

```ruby
require 'grimlocker'

client = Grimlocker::Client.new(
  base_url: 'http://127.0.0.1:36353',
  token: ENV['GRIMLOCKER_TOKEN']
)
client.unlock_vault!('master-password')

# Passwords
client.passwords.each { |p| puts "#{p.title}: #{p.username}" }
id = client.create_password!(Grimlocker::PasswordEntry.new('', 'GitHub', 'me@example.com', 's3cr3t', 'https://github.com', ''))

# File vault
listing = client.list_folder
folder  = client.create_folder!('Documents')
file    = client.upload_file!(File.binread('secret.pdf'), filename: 'secret.pdf', folder_id: folder.id) { |p| puts "#{p[:bytes_sent] * 100 / p[:total_bytes]}%" }

# Sync + Audit
sync   = client.sync_status
events = client.audit_events(n: 20)

# Workspaces
client.workspaces.each { |w| puts w.name }
```
