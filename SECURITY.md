# Security Policy

## Supported versions

Security fixes are applied to the latest released version. Older releases may not receive backports before the project establishes a long-term support policy.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting feature for this repository. Include the affected version, reproduction steps, impact, and any suggested remediation.

Do not disclose a suspected vulnerability in a public issue, pull request, or discussion. If private vulnerability reporting is unavailable, open a public issue containing no sensitive details and ask the maintainers for a private reporting channel.

The maintainers will acknowledge a report, assess severity and scope, coordinate a fix, and credit reporters who want attribution.

## Security boundary

Pixel Agent Office is intentionally restricted to localhost. It may display delegated prompts and command output, so do not proxy or expose it to another network. ForgeCode credentials and provider credentials remain managed by ForgeCode and are not stored by Agent Workforce.
