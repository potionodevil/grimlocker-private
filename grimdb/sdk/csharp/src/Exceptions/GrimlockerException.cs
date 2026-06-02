namespace Grimlocker;

public class GrimlockerException : Exception
{
    public int ErrorCode { get; }

    public GrimlockerException(string message, int errorCode = 0) : base(message)
        => ErrorCode = errorCode;

    public GrimlockerException(string message, Exception inner, int errorCode = 0)
        : base(message, inner) => ErrorCode = errorCode;

    public static string NameOf(int code) => code switch
    {
        -1   => "BUS_ERROR",
        -2   => "STORAGE_ERROR",
        -3   => "NOT_FOUND",
        -10  => "ENTRY_NOT_FOUND",
        -20  => "CATEGORY_ERROR",
        -30  => "CREATE_FAILED",
        -31  => "UPDATE_FAILED",
        -32  => "DELETE_FAILED",
        -100 => "PROTOCOL_ERROR",
        -101 => "AUTH_REQUIRED",
        -102 => "PERMISSION_DENIED",
        -103 => "INVALID_REQUEST",
        -104 => "TIMEOUT",
        _    => "UNKNOWN",
    };
}
