# Project state is derived from links

Project activations will not be represented by a committed manifest. The project-local managed-skill soft links themselves are the authoritative activation state, preserving a clean repository while allowing unmanaged project skills to coexist.

## Considered Options

- Commit a project activation manifest
- Infer activation from managed-skill soft links
