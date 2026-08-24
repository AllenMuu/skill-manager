# Use Go for the CLI and reserve React and Wails for later interfaces

Skill Manager will implement its first release as a Go CLI. The domain model, application services, filesystem operations, and operation journal will be independent from command and terminal presentation packages.

The intended later evolution is a React and TypeScript WebUI backed by the same Go application services, followed by a Wails macOS package that reuses the React interface and Go core. The first release will not include an HTTP service, frontend toolchain, browser interface, or Wails dependency. Each later phase will receive its own specification before implementation.

## Considered Options

- Use Node.js and TypeScript for the CLI, service, and Web UI
- Use Go for the CLI and core with React for the Web UI, preserving a Wails-compatible boundary
- Use SwiftUI and AppKit for a macOS-only application

The staged Go, React, and Wails route was selected because it gives the first release a small, single-executable scope while preserving a reliable filesystem core for later interfaces. It avoids speculative GUI infrastructure without making terminal behavior the place where domain rules live.
