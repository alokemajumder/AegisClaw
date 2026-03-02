# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in AegisClaw, please report it responsibly.

**DO NOT** open a public GitHub issue for security vulnerabilities.

### How to Report

1. Email: Send details to the project maintainers via a private channel
2. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

### What to Expect

- Acknowledgment within 48 hours
- Assessment and response within 7 days
- We will work with you to understand and address the issue
- Credit will be given (unless you prefer otherwise)

## Supported Versions

| Version | Supported |
|---------|----------|
| latest main | Yes |
| older releases | Best effort |

## Security Design

AegisClaw is designed with security as a core principle. See [Security Model](docs/security-model.md) for details on:

- Governance tiers and safety controls
- Authentication and authorization (JWT + RBAC + SSO)
- Immutable audit trails and signed receipts
- Data encryption (in transit and at rest)
- Network isolation and egress controls
- Connector credential handling

## Responsible Use

AegisClaw is designed for **authorized security validation** within organizations. It includes strict safety controls (tier enforcement, allowlists, kill switches) to prevent misuse. Users are responsible for:

- Configuring appropriate target allowlists
- Setting proper governance tiers
- Obtaining authorization before running validations
- Following their organization's security policies
