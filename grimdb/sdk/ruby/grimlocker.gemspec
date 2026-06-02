require_relative 'lib/grimlocker/version'

Gem::Specification.new do |spec|
  spec.name          = 'grimlocker'
  spec.version       = Grimlocker::VERSION
  spec.authors       = ['Grimlocker']
  spec.summary       = 'Ruby SDK for the Grimlocker Zero-Trust Vault'
  spec.description   = 'Synchronous HTTP client for the Grimlocker daemon. Full API coverage: entries, file vault, workspaces, sync, audit.'
  spec.license       = 'UNLICENSED'
  spec.required_ruby_version = '>= 3.1'
  spec.files         = Dir['lib/**/*', 'README.md']
  spec.require_paths = ['lib']
  spec.add_development_dependency "minitest", "~> 5.0"
  spec.add_development_dependency "rake", "~> 13.0"
end
