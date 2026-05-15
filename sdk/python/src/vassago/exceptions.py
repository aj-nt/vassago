"""Custom exceptions for the Vassago Python SDK."""


class VassagoError(Exception):
    """Base exception for all Vassago SDK errors."""


class ConnectionError(VassagoError):
    """Failed to connect to the Vassago daemon."""


class NotFoundError(VassagoError):
    """Requested resource was not found."""


class ValidationError(VassagoError):
    """Invalid input parameters."""


class TimeoutError(VassagoError):
    """Operation timed out."""


class AuthenticationError(VassagoError):
    """Authentication failed (invalid or missing API key)."""


class ServiceUnavailableError(VassagoError):
    """The daemon is not reachable or returned UNAVAILABLE."""