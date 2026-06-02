# frozen_string_literal: true
module Grimlocker
  class GrimlockerError < StandardError
    attr_reader :error_code
    def initialize(msg = 'Grimlocker error', code = 0)
      super(msg)
      @error_code = code
    end

    def self.name_of(code)
      { -1 => 'BUS_ERROR', -2 => 'STORAGE_ERROR', -3 => 'NOT_FOUND',
        -10 => 'ENTRY_NOT_FOUND', -20 => 'CATEGORY_ERROR',
        -30 => 'CREATE_FAILED', -31 => 'UPDATE_FAILED', -32 => 'DELETE_FAILED',
        -100 => 'PROTOCOL_ERROR', -101 => 'AUTH_REQUIRED',
        -102 => 'PERMISSION_DENIED', -103 => 'INVALID_REQUEST' }[code] || 'UNKNOWN'
    end
  end
end
