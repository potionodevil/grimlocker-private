class GrimlockerException implements Exception {
  final int code;
  final String message;

  GrimlockerException(this.code, this.message);

  @override
  String toString() => 'GrimlockerException($code): $message';

  static String nameOf(int code) => switch (code) {
    -1   => 'BUS_ERROR',
    -2   => 'STORAGE_ERROR',
    -3   => 'NOT_FOUND',
    -10  => 'ENTRY_NOT_FOUND',
    -20  => 'CATEGORY_ERROR',
    -30  => 'CREATE_FAILED',
    -31  => 'UPDATE_FAILED',
    -32  => 'DELETE_FAILED',
    -100 => 'PROTOCOL_ERROR',
    -101 => 'AUTH_REQUIRED',
    -102 => 'PERMISSION_DENIED',
    -103 => 'INVALID_REQUEST',
    -104 => 'TIMEOUT',
    _    => 'UNKNOWN',
  };
}

class CircuitBreakerOpenException implements Exception {
  @override
  String toString() => 'CircuitBreakerOpenException: Circuit breaker is open';
}
