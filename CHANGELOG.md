# Changelog

All notable changes to Agent Workforce will be documented here.

The project follows Keep a Changelog and intends to use Semantic Versioning after the first public release.

## [Unreleased]

### Added

- Coordinator-driven ForgeCode sessions and MCP specialist tools.
- Local Pixel Agent Office for workforce state and Forge output.
- Agent installation, activation, validation, editing, and synchronization commands.
- Reproducible CI and tagged release archives for Linux and macOS.

### Security

- Restrict Pixel Agent Office to loopback hosts and same-origin browser requests.
- Bound Office request bodies and MCP stdio messages.
- Validate agent identifiers and manifest paths before deletion.
