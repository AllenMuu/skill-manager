# Use the existing shared skill library

The initial authoritative library is `~/.agents/skills`, which already contains the local reusable skills and is already linked by Claude. Skill Manager will keep this location configurable so the library can be relocated later without changing project activations.

## Considered Options

- Adopt `~/.agents/skills` as the initial library
- Create a new library and migrate the existing collection
