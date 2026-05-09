"""Custom exceptions for the Mnemosyne Python SDK."""


class MnemosyneError(Exception):
    """Base exception for all Mnemosyne SDK errors."""


class ConnectionError(MnemosyneError):
    """Failed to connect to the Mnemosyne daemon."""


class NotFoundError(MnemosyneError):
    """Requested resource was not found."""


class ValidationError(MnemosyneError):
    """Invalid input parameters."""


class TimeoutError(MnemosyneError):
    """Operation timed out."""


class AuthenticationError(MnemosyneError):
    """Authentication failed (invalid or missing API key)."""


class ServiceUnavailableError(MnemosyneError):
    """The daemon is not reachable or returned UNAVAILABLE."""